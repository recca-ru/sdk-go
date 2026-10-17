package cpatest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.recca.ru/sdk/cpa"
)

// apiPrefix — версионированный префикс, который подставляет транспорт клиента.
const apiPrefix = "/federation/v1"

// handle записывает обращение, отдаёт заготовленный отказ либо маршрутизирует.
func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	path := strings.TrimPrefix(r.URL.Path, apiPrefix)

	s.mu.Lock()
	s.requests = append(s.requests, Request{
		Method: r.Method, Path: path, Query: r.URL.Query(), Body: body,
	})
	requestID := "req_cpatest_" + strconv.Itoa(len(s.requests))
	pending := s.pending
	// ⚠ Одноразовая заготовка снимается ЗДЕСЬ, до ответа: иначе повторный вызов
	// получил бы тот же отказ, и «ровно один раз» превратилось бы в «до отмены».
	// Удержанная (HoldError) не снимается — её снимает только ClearError.
	if !s.held {
		s.pending = nil
	}
	s.mu.Unlock()

	if pending != nil {
		s.writeError(w, requestID, *pending, "cpatest: отказ заготовлен PrepareError")
		return
	}

	s.route(w, r, path, body, requestID)
}

// route — все восемнадцать путей контракта.
//
// ⚠ Разбор руками, а не http.ServeMux: стенду нужен ПУТЬ ОТ ВЕРСИИ в записи
// обращений и однозначный ответ на незнакомый путь. Незнакомый путь обязан быть
// громким — молчаливый 404 читался бы как «дверь есть, данных нет».
func (s *Server) route(w http.ResponseWriter, r *http.Request, path string, body []byte, requestID string) {
	segments := strings.Split(strings.Trim(path, "/"), "/")

	switch {
	case path == "/cpa/self" && r.Method == http.MethodGet:
		self := fixture("self.json")
		if s.environment != "" {
			self = patch(self, map[string]any{"environment": s.environment})
		}
		s.writeData(w, requestID, self, http.StatusOK, nil)

	case path == "/cpa/self/webhook" && r.Method == http.MethodPut:
		s.writeData(w, requestID, s.webhookOf(body), http.StatusOK, nil)

	case path == "/cpa/partners" && r.Method == http.MethodGet:
		s.writeList(w, requestID, []json.RawMessage{fixture("partner.json")})

	case path == "/cpa/partners" && r.Method == http.MethodPost:
		s.writeData(w, requestID, s.partnerOf(body), http.StatusCreated, nil)

	// /cpa/partners/{external_id}[/balance]
	case len(segments) == 3 && segments[0] == "cpa" && segments[1] == "partners" && r.Method == http.MethodGet:
		s.writeData(w, requestID, fixture("partner.json"), http.StatusOK, nil)

	case len(segments) == 4 && segments[1] == "partners" && segments[3] == "balance" && r.Method == http.MethodGet:
		s.writeData(w, requestID, s.balance(), http.StatusOK, nil)

	case path == "/cpa/offers" && r.Method == http.MethodGet:
		if !s.advertiserViewAllowed(w, r, requestID) {
			return
		}
		s.writeList(w, requestID, []json.RawMessage{s.offerFor(r)})

	case len(segments) == 3 && segments[1] == "offers" && r.Method == http.MethodGet:
		if !s.advertiserViewAllowed(w, r, requestID) {
			return
		}
		s.writeData(w, requestID, s.offerFor(r), http.StatusOK, nil)

	case len(segments) == 4 && segments[1] == "offers" && segments[3] == "creatives" && r.Method == http.MethodGet:
		var creatives []json.RawMessage
		if err := json.Unmarshal(fixture("creatives.json"), &creatives); err != nil {
			s.writeError(w, requestID, cpa.CodeInternalError, "cpatest: фикстура материалов не разобрана")
			return
		}
		s.writeList(w, requestID, creatives)

	case path == "/cpa/links" && r.Method == http.MethodPost:
		s.writeData(w, requestID, fixture("link.json"), http.StatusCreated, nil)

	case path == "/cpa/clicks" && r.Method == http.MethodPost:
		s.acceptClick(w, body, requestID)

	case path == "/cpa/conversions" && r.Method == http.MethodGet:
		s.writeList(w, requestID, []json.RawMessage{stripTriple(fixture("conversion.json"))})

	case path == "/cpa/conversions" && r.Method == http.MethodPost:
		s.acceptConversion(w, body, requestID)

	case len(segments) == 3 && segments[1] == "conversions" && r.Method == http.MethodGet:
		s.conversionAt(w, r, segments[2], requestID)

	case len(segments) == 4 && segments[1] == "conversions" && segments[3] == "cancel" && r.Method == http.MethodPost:
		s.cancelConversion(w, body, segments[2], requestID)

	case path == "/cpa/payouts" && r.Method == http.MethodGet:
		s.writeList(w, requestID, []json.RawMessage{fixture("payout.json")})

	case path == "/cpa/payouts" && r.Method == http.MethodPost:
		s.acceptPayout(w, body, requestID)

	case path == "/cpa/statistics" && r.Method == http.MethodGet:
		var rows []json.RawMessage
		if err := json.Unmarshal(fixture("statistics.json"), &rows); err != nil {
			s.writeError(w, requestID, cpa.CodeInternalError, "cpatest: фикстура отчёта не разобрана")
			return
		}
		s.writeList(w, requestID, rows)

	default:
		// ⚠⚠ Громко и с путём в сообщении: незнакомый путь означает, что либо SDK
		// стучится не туда, либо стенд отстал от контракта, — и оба случая надо
		// увидеть сразу, а не через час разбора пустого списка.
		s.writeError(w, requestID, cpa.CodeInternalError,
			fmt.Sprintf("cpatest: путь %s %s стендом не объявлен", r.Method, path))
	}
}

// ── формирование ответа ──────────────────────────────────────────────────────

func (s *Server) writeData(w http.ResponseWriter, requestID string, data json.RawMessage, status int, meta map[string]any) {
	envelope := map[string]any{
		"success": true,
		"data":    data,
		"meta":    mergeMeta(requestID, meta),
	}
	writeJSON(w, status, envelope)
}

// writeList отдаёт страницу вместе с блоком пагинации: у страницы это часть
// ОТВЕТА, и клиент читает её из meta.
func (s *Server) writeList(w http.ResponseWriter, requestID string, rows []json.RawMessage) {
	s.writeData(w, requestID, mustMarshal(rows), http.StatusOK, map[string]any{
		"total": len(rows), "count": len(rows), "offset": 0, "limit": 20,
		"has_more": false,
		// ⚠ truncated приходит всегда, включая false: «мы посмотрели всё» — это
		// утверждение, а не отсутствие оговорки.
		"truncated": false,
	})
}

// writeError — отказ без поимённого поля. Дверь несёт details всегда (null,
// когда поля нет), и стенд повторяет форму конверта дословно: клиент,
// ожидающий ключ, не должен видеть его то есть, то нет.
func (s *Server) writeError(w http.ResponseWriter, requestID string, code cpa.ErrorCode, message string) {
	s.writeErrorDetails(w, requestID, code, message, nil)
}

// writeErrorDetails — отказ с поимённым полем: дверь на отказах про вход
// называет в details виновника (окно occurred_at — { field: "occurred_at" }).
func (s *Server) writeErrorDetails(w http.ResponseWriter, requestID string, code cpa.ErrorCode, message string, details map[string]any) {
	status, known := cpa.HTTPStatusFor(code)
	if !known {
		// Код вне реестра — ошибка вызова PrepareError, и она обязана быть видна.
		status = http.StatusInternalServerError
		message = "cpatest: код " + string(code) + " не принадлежит реестру контракта"
	}
	// nilany, а не пустая карта: без поимённого поля дверь шлёт details: null,
	// и пустой объект {} читался бы как «поле есть, но не названо».
	var nilable any
	if details != nil {
		nilable = details
	}
	writeJSON(w, status, map[string]any{
		"success": false,
		"error":   map[string]any{"code": string(code), "message": message, "details": nilable},
		"meta":    mergeMeta(requestID, nil),
	})
}

func mergeMeta(requestID string, extra map[string]any) map[string]any {
	meta := map[string]any{"request_id": requestID}
	for k, v := range extra {
		meta[k] = v
	}
	return meta
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	raw := mustMarshal(value)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-Id", requestIDOf(value))
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}

// requestIDOf достаёт идентификатор из конверта, чтобы он уехал и заголовком:
// клиент предпочитает заголовок, и не прислать его значило бы не проверить этот
// путь вовсе.
func requestIDOf(value any) string {
	envelope, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	meta, ok := envelope["meta"].(map[string]any)
	if !ok {
		return ""
	}
	id, _ := meta["request_id"].(string)
	return id
}

func mustMarshal(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic("cpatest: ответ не собрался: " + err.Error())
	}
	return raw
}

// ── подгонка фикстур под запрос ──────────────────────────────────────────────
//
// ⚠ Ровно подстановка присланных значений, и ничего больше: стенд обязан
// вернуть ТО, о чём его спросили, иначе проверка «ушло то, что ждали» была бы
// зелена на любом запросе.

func patch(raw json.RawMessage, set map[string]any) json.RawMessage {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		panic("cpatest: фикстура не объект: " + err.Error())
	}
	for key, value := range set {
		object[key] = mustMarshal(value)
	}
	return mustMarshal(object)
}

func withStatus(raw json.RawMessage, status string) json.RawMessage {
	return patch(raw, map[string]any{"status": status})
}

// stripTriple убирает из карточки поля, которых НЕТ в проекции списка. Отдавать
// их в списке значило бы проверять клиента на ответе, которого дверь не даёт.
func stripTriple(raw json.RawMessage) json.RawMessage {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		panic("cpatest: фикстура не объект: " + err.Error())
	}
	for _, key := range []string{"revenue_rub", "margin_rub", "agent_leg", "fraud_flags"} {
		delete(object, key)
	}
	return mustMarshal(object)
}

func (s *Server) webhookOf(body []byte) json.RawMessage {
	var in struct {
		URL          string `json:"url"`
		RotateSecret bool   `json:"rotate_secret"`
	}
	_ = json.Unmarshal(body, &in)

	set := map[string]any{"url": in.URL}
	if !in.RotateSecret {
		// ⚠ Секрет показывается при установке и при ротации. Здесь он всегда
		// присутствует — стенд структурный, и «когда именно его показывают»
		// доказывает песочница.
		set["secret"] = "whsec_shown_once"
	} else {
		set["secret"] = "whsec_rotated"
	}
	return patch(fixture("webhook.json"), set)
}

func (s *Server) partnerOf(body []byte) json.RawMessage {
	var in struct {
		ExternalID   string `json:"external_id"`
		Role         string `json:"role"`
		PartnerState string `json:"partner_state"`
		// ⚠ Открепление (null) и «не меняем» (ключа нет) стенд различать не
		// обязан: состояния партнёров он не держит, и в обоих случаях отвечает
		// фикстурой, где рекрутёра нет. Что null действительно уходит в тело,
		// доказывает запись обращения, а не ответ стенда.
		Recruiter string `json:"recruiter_external_id"`
	}
	_ = json.Unmarshal(body, &in)

	set := map[string]any{"external_id": in.ExternalID, "role": in.Role}
	if in.PartnerState != "" {
		set["partner_state"] = in.PartnerState
	}
	if in.Recruiter != "" {
		set["recruiter_external_id"] = in.Recruiter
	}
	return patch(fixture("partner.json"), set)
}

// offerFor выбирает проекцию по параметру view.
//
// ⚠⚠ Умолчание здесь партнёрское — как у двери. Отдай по умолчанию проекцию
// рекламодателя, и тест, забывший параметр, зеленел бы на ответе, который живая
// дверь не даёт.
func (s *Server) offerFor(r *http.Request) json.RawMessage {
	if r.URL.Query().Get("view") == "advertiser" {
		return fixture("offer_advertiser.json")
	}
	return fixture("offer_partner.json")
}

// advertiserViewAllowed держит требование двери к view=advertiser
// (WV-0828 Д2R1-07, с 02.10): параметр advertiser_id обязан явиться — и
// обязан называть рекламодателя ЭТОГО оффера, у стенда он один (adv_1):
// чужой идентификатор открывал бы ставки чужой карточки перебором. Ответы —
// двери дословно: 400 INVALID_INPUT с поимённым полем, чужой — 404
// ADVERTISER_NOT_FOUND (текст — реестра, его же шлёт дверь). Правило держат
// ОБЕ двери чтения: каталог и карточка.
//
// ⚠⚠ До 03.10 стенд этого требования не моделировал — и дефект v0.2.1
// (GetForAdvertiser не шлёт advertiser_id) стоял зелёным против стенда,
// оракула и мутаций; нашла его живая приёмка против песочницы.
func (s *Server) advertiserViewAllowed(w http.ResponseWriter, r *http.Request, requestID string) bool {
	if r.URL.Query().Get("view") != "advertiser" {
		return true
	}
	advertiserID := r.URL.Query().Get("advertiser_id")
	if advertiserID == "" {
		s.writeErrorDetails(w, requestID, cpa.CodeInvalidInput,
			"advertiser_id is required when view=advertiser",
			map[string]any{"field": "advertiser_id"})
		return false
	}
	if advertiserID != "adv_1" {
		s.writeErrorDetails(w, requestID, cpa.CodeAdvertiserNotFound,
			"Рекламодателя с таким `advertiser_id` в этой сети нет.",
			map[string]any{"field": "advertiser_id"})
		return false
	}
	return true
}

// ── идемпотентность и гейт пары: логика стенда ───────────────────────────────

// multipleGoals — флаг «несколько конверсий с одного клика» по коду цели,
// прочитанный из партнёрской проекции оффера стенда. Флаг приходит в ОБЕИХ
// проекциях (контракт 2026-10-02), стенду для решения гейта достаточно одной.
var multipleGoals = sync.OnceValue(func() map[string]bool {
	var offer struct {
		Goals []struct {
			Code             string `json:"code"`
			MultiplePerClick bool   `json:"multiple_per_click"`
		} `json:"goals"`
	}
	if err := json.Unmarshal(fixture("offer_partner.json"), &offer); err != nil {
		panic("cpatest: цели партнёрской проекции не разобраны: " + err.Error())
	}
	flags := make(map[string]bool, len(offer.Goals))
	for _, g := range offer.Goals {
		flags[g.Code] = g.MultiplePerClick
	}
	return flags
})

// goalAllowsMultiple — включено ли у цели «несколько конверсий с одного клика».
// Цели вне словаря — false: стенд структурный, и цель, которой он не знает,
// консервативно одиночна, а не молча проводит вторую конверсию.
func goalAllowsMultiple(goal string) bool { return multipleGoals()[goal] }

// occurredAtWindowDepth — окно времени события двери (WV-0828 Ж-15):
// конверсии принимаются не глубже 90 дней. Глубже — бэкдейтинг клал
// конверсию в чужой день отчёта и ел кап того дня. Граница ВКЛЮЧИТЕЛЬНА:
// ровно 90 дней назад — ещё принимается (дверь: parseOccurredAt в Recca).
const occurredAtWindowDepth = 90 * 24 * time.Hour

// occurredAtTooOld — событие глубже окна. Без метки — false: дверь живёт
// прежней семантикой. Будущее здесь НЕ проверяется: его дверь бракует
// INVALID_INPUT'ом, и это другой код, не OCCURRED_AT_OUT_OF_WINDOW.
func (s *Server) occurredAtTooOld(at *time.Time) bool {
	if at == nil {
		return false
	}
	return at.Before(s.now().Add(-occurredAtWindowDepth))
}

// now — время стенда; замораживается внутренним тестом границы окна: на
// настоящих часах «ровно 90 дней» мигало бы на микросекундах между
// конструкцией значения и разбором запроса.
func (s *Server) now() time.Time {
	s.mu.Lock()
	clock := s.clock
	s.mu.Unlock()
	if clock != nil {
		return clock()
	}
	return time.Now()
}

// normalizeGoal — общий ключ цели для приёма, чтения и отмены: нет поля и
// пустая строка — «1», как normalizeGoal в Recca. Разойдись адресация приёма и
// чтения, карточка не нашлась бы на конверсии, только что созданной.
func normalizeGoal(goal *string) string {
	if goal != nil && strings.TrimSpace(*goal) != "" {
		return strings.TrimSpace(*goal)
	}
	return "1"
}

func (s *Server) acceptConversion(w http.ResponseWriter, body []byte, requestID string) {
	var in struct {
		ClickID    string           `json:"click_id"`
		ExternalID string           `json:"external_id"`
		Goal       *string          `json:"goal"`
		Status     string           `json:"status"`
		SubID      string           `json:"sub_id"`
		SumRUB     *json.RawMessage `json:"sum_rub"`
		OccurredAt *time.Time       `json:"occurred_at"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		s.writeError(w, requestID, cpa.CodeInvalidInput, "cpatest: тело не разобрано")
		return
	}

	// Окно времени события (WV-0828 Ж-15) — до идемпотентности, как у двери:
	// вход вне окна не принимается вовсе, и повтор старого заказа не может
	// пройти «по памяти». Текст — двери, дословно; details называет поле
	// поимённо, тоже как дверь.
	if s.occurredAtTooOld(in.OccurredAt) {
		s.writeErrorDetails(w, requestID, cpa.CodeOccurredAtOutOfWindow,
			"occurred_at is older than the 90-day window",
			map[string]any{"field": "occurred_at"})
		return
	}

	goal := normalizeGoal(in.Goal)
	key := in.ExternalID + "\x00" + goal

	s.mu.Lock()
	stored, repeat := s.conversions[key]
	s.mu.Unlock()

	if repeat {
		// ⚠⚠ 200, а не 201: повтор — успех, и это то самое место, где ломается
		// клиент, считающий успехом только 201.
		status := http.StatusOK
		s.mu.Lock()
		if s.repeatStatus != 0 {
			status = s.repeatStatus
		}
		s.mu.Unlock()
		s.writeData(w, requestID, stored, status, nil)
		return
	}

	// Гейт пары «клик + цель» (WV-0827): без включённой у цели «несколько
	// конверсий с одного клика» пара платится один раз. Идемпотентность уже
	// отработала выше — до гейта, как у двери: повтор СВОЕГО заказа успех при
	// любом состоянии пары. Отказ называет заказ, занявший пару: интегратор
	// решает по номеру, чей это повтор. Текст — двери, дословно.
	if in.ClickID != "" {
		pair := in.ClickID + "\x00" + goal
		s.mu.Lock()
		occupier, taken := s.pairGates[pair]
		ignore := s.ignorePairGate
		s.mu.Unlock()
		if !ignore && taken && occupier != in.ExternalID && !goalAllowsMultiple(goal) {
			s.writeError(w, requestID, cpa.CodeAlreadyConverted, fmt.Sprintf(
				"click has already converted for this goal by external_id %q; "+
					"one click pays once per goal unless the goal allows multiple conversions per click",
				occupier))
			return
		}
	}

	set := map[string]any{"external_id": in.ExternalID, "goal": goal}
	if in.SubID != "" {
		set["sub_id"] = in.SubID
	}
	record := patch(fixture("conversion.json"), set)
	if in.SumRUB != nil {
		record = patch(record, map[string]any{"sum_rub": *in.SumRUB})
	}

	record = s.applyTamper(record)

	s.mu.Lock()
	s.conversions[key] = record
	if in.ClickID != "" {
		s.convClick[key] = in.ClickID
		s.pairGates[in.ClickID+"\x00"+goal] = in.ExternalID
	}
	s.mu.Unlock()

	s.writeData(w, requestID, record, http.StatusCreated, nil)
}

// acceptClick выдаёт переход. Ключ отправителя идемпотентен, как у двери:
// повтор ключа возвращает ТОТ ЖЕ клик (200), новый ключ — новый клик (201), без
// ключа каждый вызов — новый переход. Раньше стенд раздавал всем один
// фикстурный click_id, и пара «клик + цель» была неразличима между вызовами.
func (s *Server) acceptClick(w http.ResponseWriter, body []byte, requestID string) {
	var in struct {
		ClientClickID string `json:"client_click_id"`
	}
	_ = json.Unmarshal(body, &in)

	s.mu.Lock()
	clickID, repeat := s.clicks[in.ClientClickID]
	if !repeat {
		s.clickID++
		clickID = "clk_cpatest_" + strconv.Itoa(s.clickID)
		if in.ClientClickID != "" {
			s.clicks[in.ClientClickID] = clickID
		}
	}
	s.mu.Unlock()

	status := http.StatusCreated
	if repeat {
		status = http.StatusOK
	}
	// redirect_url фикстуры несёт ЕЁ клик; дверь строит адрес на выданный
	// click_id, и стенд подставляет его, чтобы ответ не ссылался на чужой клик.
	raw := fixture("click.json")
	var clickDoc struct {
		ClickID     string `json:"click_id"`
		RedirectURL string `json:"redirect_url"`
	}
	_ = json.Unmarshal(raw, &clickDoc)
	set := map[string]any{"click_id": clickID}
	if clickDoc.RedirectURL != "" && clickDoc.ClickID != "" {
		set["redirect_url"] = strings.Replace(clickDoc.RedirectURL, clickDoc.ClickID, clickID, 1)
	}
	s.writeData(w, requestID, patch(raw, set), status, nil)
}

// conversionAt отдаёт карточку СОХРАННЁННОЙ строки по тройке «external_id +
// цель»: цель — query-параметр. Раньше стенд отвечал фикстурой на любой адрес —
// «вторая конверсия жива после отмены первой» было бы недоказуемо. Нет строки —
// 404 CONVERSION_NOT_FOUND: «не заводили» и «пусто» — разные ответы.
func (s *Server) conversionAt(w http.ResponseWriter, r *http.Request, externalID, requestID string) {
	goal := r.URL.Query().Get("goal")
	s.mu.Lock()
	stored, ok := s.conversions[externalID+"\x00"+normalizeGoal(&goal)]
	s.mu.Unlock()
	if !ok {
		s.writeError(w, requestID, cpa.CodeConversionNotFound,
			fmt.Sprintf("cpatest: конверсия %s на цели %q стендом не заведена", externalID, normalizeGoal(&goal)))
		return
	}
	s.writeData(w, requestID, s.applyTamper(stored), http.StatusOK, nil)
}

// cancelConversion гасит сохранённую строку: статус declined. Если эта строка
// держала пару «клик + цель», пара освобождается — дверь ищет занявший заказ
// по статусам hold/approved (WV-0827), значит отмена возвращает паре
// платежеспособность. Повторная отмена идемпотентна: строка уже declined.
func (s *Server) cancelConversion(w http.ResponseWriter, body []byte, externalID, requestID string) {
	var in struct {
		Goal *string `json:"goal"`
	}
	_ = json.Unmarshal(body, &in)
	goal := normalizeGoal(in.Goal)
	key := externalID + "\x00" + goal

	s.mu.Lock()
	stored, ok := s.conversions[key]
	if ok {
		stored = withStatus(stored, "declined")
		s.conversions[key] = stored
		if s.pairGates[s.convClick[key]+"\x00"+goal] == externalID {
			delete(s.pairGates, s.convClick[key]+"\x00"+goal)
		}
	}
	s.mu.Unlock()
	if !ok {
		s.writeError(w, requestID, cpa.CodeConversionNotFound,
			fmt.Sprintf("cpatest: конверсия %s на цели %q стендом не заведена", externalID, goal))
		return
	}
	s.writeData(w, requestID, stored, http.StatusOK, nil)
}

func (s *Server) acceptPayout(w http.ResponseWriter, body []byte, requestID string) {
	var in struct {
		PartnerExternalID string `json:"partner_external_id"`
		AmountRUB         int64  `json:"amount_rub"`
		PeriodFrom        string `json:"period_from"`
		PeriodTo          string `json:"period_to"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		s.writeError(w, requestID, cpa.CodeInvalidInput, "cpatest: тело не разобрано")
		return
	}

	// ⚠⚠ Ключ — вся четвёрка, БЕЗ суммы: сумма сверяется отдельно. Включи её в
	// ключ — и повтор с другой суммой завёл бы вторую заявку вместо отказа, то
	// есть стенд перестал бы отличать идемпотентность от дубля.
	key := in.PartnerExternalID + "\x00" + in.PeriodFrom + "\x00" + in.PeriodTo

	s.mu.Lock()
	stored, repeat := s.payouts[key]
	refuse := s.refusePayouts
	s.mu.Unlock()

	switch {
	case refuse:
		s.writeError(w, requestID, cpa.CodeInsufficientBalance,
			fmt.Sprintf("cpatest: доступного остатка не хватает на %d ₽", in.AmountRUB))
		return
	case repeat && stored.amount != in.AmountRUB:
		s.writeError(w, requestID, cpa.CodeIdempotentMismatch,
			fmt.Sprintf("заявка с этим ключом создана на %d ₽, а пришло %d ₽", stored.amount, in.AmountRUB))
		return
	case repeat:
		s.writeData(w, requestID, stored.body, http.StatusOK, nil)
		return
	}

	record := patch(fixture("payout.json"), map[string]any{
		"partner_external_id": in.PartnerExternalID,
		"amount_rub":          in.AmountRUB,
		"period_from":         in.PeriodFrom,
		"period_to":           in.PeriodTo,
	})

	s.mu.Lock()
	s.payouts[key] = payoutRecord{amount: in.AmountRUB, body: record}
	s.mu.Unlock()

	s.writeData(w, requestID, record, http.StatusCreated, nil)
}

// balance — баланс партнёра: фикстура либо, под внутренней ручкой canRequest,
// её подмена.
func (s *Server) balance() json.RawMessage {
	s.mu.Lock()
	canRequest := s.canRequest
	s.mu.Unlock()

	raw := fixture("partner_balance.json")
	switch {
	case canRequest == nil:
		return raw
	case *canRequest:
		return patch(raw, map[string]any{"can_request": true})
	default:
		return patch(raw, map[string]any{"can_request": false, "available_rub": 0})
	}
}

// bodyOf — для утверждений в тестах потребителя: тело обращения как строка.
func (r Request) BodyString() string { return string(bytes.TrimSpace(r.Body)) }

// applyTamper пропускает карточку через порчу, если её завёл внутренний тест.
func (s *Server) applyTamper(raw json.RawMessage) json.RawMessage {
	s.mu.Lock()
	tamper := s.tamper
	s.mu.Unlock()
	if tamper == nil {
		return raw
	}
	return tamper(raw)
}
