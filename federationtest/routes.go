package federationtest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"go.recca.ru/sdk/federation"
)

func readAll(r *http.Request) []byte {
	body, _ := io.ReadAll(r.Body)
	return body
}

// fixture читает тело ответа из testdata.
//
// ⚠ Паника, а не пустой ответ: отсутствие фикстуры — ошибка стенда, а молчащий
// стенд превратил бы её в непонятный отказ разбора у потребителя.
func fixture(name string) json.RawMessage {
	raw, err := fixtures.ReadFile("testdata/fixtures/" + name)
	if err != nil {
		panic("federationtest: нет фикстуры " + name + ": " + err.Error())
	}
	return json.RawMessage(raw)
}

// patch правит поля фикстуры, не переписывая её целиком.
func patch(data json.RawMessage, fields map[string]any) json.RawMessage {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		panic("federationtest: фикстура не объект: " + err.Error())
	}
	for k, v := range fields {
		raw, err := json.Marshal(v)
		if err != nil {
			panic("federationtest: поле " + k + " не сериализуется: " + err.Error())
		}
		obj[k] = raw
	}
	out, err := json.Marshal(obj)
	if err != nil {
		panic("federationtest: правленая фикстура не собралась: " + err.Error())
	}
	return out
}

// dropField убирает поле из фикстуры целиком — так отвечает дверь, которая о
// нём не знает.
func dropField(data json.RawMessage, name string) json.RawMessage {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		panic("federationtest: фикстура не объект: " + err.Error())
	}
	delete(obj, name)
	out, err := json.Marshal(obj)
	if err != nil {
		panic("federationtest: урезанная фикстура не собралась: " + err.Error())
	}
	return out
}

// bodyField достаёт строковое поле присланного тела.
func bodyField(body []byte, name string) string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return ""
	}
	raw, ok := obj[name]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

// route — все одиннадцать путей контракта.
//
// ⚠ Разбор руками, а не http.ServeMux: стенду нужен ПУТЬ ОТ ВЕРСИИ в записи
// обращений и однозначный ответ на незнакомый путь. Незнакомый путь обязан быть
// громким — молчаливый 404 читался бы как «дверь есть, данных нет».
func (s *Server) route(w http.ResponseWriter, r *http.Request, path string, body []byte, requestID string) {
	seg := strings.Split(strings.Trim(path, "/"), "/")
	get := r.Method == http.MethodGet
	post := r.Method == http.MethodPost

	switch {
	case path == "/self" && get:
		self := fixture("self.json")
		s.mu.Lock()
		env := s.environment
		drop := s.dropEnvironment
		s.mu.Unlock()
		// ⚠ Подмена нужна ровно одному читателю — тесту, доказывающему, что
		// набор приёмки ОТКАЗЫВАЕТСЯ идти не на песочницу. Без неё «набор
		// защищает» было бы утверждением о себе самом.
		switch {
		case drop:
			self = dropField(self, "environment")
		case env != nil:
			self = patch(self, map[string]any{"environment": *env})
		}
		s.writeData(w, requestID, path, self, http.StatusOK)

	case path == "/users/upsert" && post:
		s.writeData(w, requestID, path, fixture("user_upsert.json"), http.StatusCreated)

	case path == "/offers" && get:
		s.writeData(w, requestID, path, fixture("offers.json"), http.StatusOK)

	case path == "/leads" && post:
		s.idempotent(w, requestID, path, s.leads, bodyField(body, "external_id"),
			fixture("lead_create.json"), "idempotent")

	case path == "/conversions" && post:
		s.idempotent(w, requestID, path, s.conversions, bodyField(body, "external_id"),
			fixture("conversion_register.json"), "idempotent")

	// /conversions/{external_id}/reverse
	case len(seg) == 3 && seg[0] == "conversions" && seg[2] == "reverse" && post:
		s.writeData(w, requestID, path, fixture("conversion_reverse.json"), http.StatusOK)

	// /badges/{id}/request-public
	case len(seg) == 3 && seg[0] == "badges" && seg[2] == "request-public" && post:
		s.writeData(w, requestID, path, fixture("badge_request_public.json"), http.StatusOK)

	// /users/{id}
	case len(seg) == 2 && seg[0] == "users" && get:
		s.writeData(w, requestID, path, fixture("user_get.json"), http.StatusOK)

	// /users/{id}/leads
	case len(seg) == 3 && seg[0] == "users" && seg[2] == "leads" && get:
		s.writeData(w, requestID, path, fixture("user_leads.json"), http.StatusOK)

	// /users/{id}/tree
	case len(seg) == 3 && seg[0] == "users" && seg[2] == "tree" && get:
		s.writeData(w, requestID, path, fixture("user_tree.json"), http.StatusOK)

	// /users/{id}/bonus-balance
	case len(seg) == 3 && seg[0] == "users" && seg[2] == "bonus-balance" && get:
		s.writeData(w, requestID, path, fixture("bonus_balance.json"), http.StatusOK)

	// /users/{id}/referral-links — идемпотентность по паре «человек ⇆ оффер»
	case len(seg) == 3 && seg[0] == "users" && seg[2] == "referral-links" && post:
		key := seg[1] + "|" + bodyField(body, "offer_id")
		s.idempotentLink(w, requestID, path, key)

	default:
		s.writeError(w, requestID, federation.CodeNotFound,
			fmt.Sprintf("federationtest: путь %s %s стенду неизвестен", r.Method, path))
	}
}

// idempotent — общая память для конверсии и заявки: первый вызов отдаёт 201,
// повтор — ТУ ЖЕ строку с поднятым признаком и 200.
//
// ⚠ Пустой различитель идёт как обычная запись, а не как повтор: иначе два
// разных события без external_id схлопнулись бы в одно, и стенд соврал бы в
// пользу клиента. Отбивать его здесь тоже незачем — это дело проверки клиента.
func (s *Server) idempotent(
	w http.ResponseWriter, requestID, path string,
	store map[string]json.RawMessage, key string,
	first json.RawMessage, flag string,
) {
	if key == "" {
		s.writeData(w, requestID, path, first, http.StatusCreated)
		return
	}

	s.mu.Lock()
	seen, repeat := store[key]
	if !repeat {
		store[key] = first
	}
	status := s.repeatStatus
	broken := s.breakIdempotency
	s.mu.Unlock()

	if !repeat {
		s.writeData(w, requestID, path, first, http.StatusCreated)
		return
	}
	if broken {
		// Повтор выдан за новую строку — признака нет, статус 201.
		s.writeData(w, requestID, path, seen, http.StatusCreated)
		return
	}
	if status == 0 {
		status = http.StatusOK
	}
	s.writeData(w, requestID, path, patch(seen, map[string]any{flag: true}), status)
}

// idempotentLink — та же память для ссылки. Отдельно, потому что признака в
// ТЕЛЕ у неё нет: повтор различается ТОЛЬКО кодом состояния, и это свойство
// живой двери, а не упрощение стенда.
func (s *Server) idempotentLink(w http.ResponseWriter, requestID, path, key string) {
	body := fixture("referral_link.json")

	s.mu.Lock()
	_, repeat := s.links[key]
	if !repeat {
		s.links[key] = body
	}
	status := s.repeatStatus
	s.mu.Unlock()

	if !repeat {
		s.writeData(w, requestID, path, body, http.StatusCreated)
		return
	}
	if status == 0 {
		status = http.StatusOK
	}
	s.writeData(w, requestID, path, body, status)
}

// writeData отдаёт успешный конверт.
//
// ⚠⚠ `meta` собирается так же, как это делает ЖИВОЙ `successResponse`
// федерации: без `request_id` и с признаком следующей страницы под именем
// `hasMore`. Положи сюда `request_id` — и клиент, читающий его из meta вместо
// заголовка, зеленел бы на стенде и падал на бою. Идентификатор уходит
// заголовком, как на живой двери.
func (s *Server) writeData(w http.ResponseWriter, requestID, path string, data json.RawMessage, status int) {
	s.mu.Lock()
	tamper := s.tamper
	s.mu.Unlock()
	if tamper != nil {
		data = tamper(path, data)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-Id", requestID)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"data":    data,
	})
}

// writeError отдаёт отказ в форме живой двери федерации.
//
// ⚠ Блока `meta` здесь НЕТ намеренно: `errorResponse` федерации его не кладёт.
// Значит request_id у отказа доступен ТОЛЬКО заголовком — ровно как на бою.
func (s *Server) writeError(w http.ResponseWriter, requestID string, code federation.ErrorCode, message string) {
	status, known := federation.HTTPStatusFor(code)
	if !known || status == 0 {
		status = http.StatusBadRequest
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-Id", requestID)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": false,
		"error": map[string]any{
			"code":    string(code),
			"message": message,
		},
	})
}
