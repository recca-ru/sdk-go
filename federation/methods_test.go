package federation_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"go.recca.ru/sdk/federation"
	"go.recca.ru/sdk/federationtest"
)

func ctx() context.Context { return context.Background() }

// ── 1. users.Upsert ──────────────────────────────────────────────────────────

func TestUsersUpsert(t *testing.T) {
	s := federationtest.New(t)
	c := s.Client(t)

	res, resp, err := c.Users.Upsert(ctx(), federation.UpsertUserInput{
		Email:         "petr@example.ru",
		FirstName:     "Пётр",
		LastName:      "Конформанс",
		EmailVerified: true,
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if res.TenantReccaID != "trid_conformance_01" || !res.IsNew || res.ClaimRequired {
		t.Errorf("Upsert разобрал не то: %+v", res)
	}
	if res.Profile.Name != "Пётр Конформанс" {
		t.Errorf("профиль разобран не то: %+v", res.Profile)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("статус %d, ждали 201", resp.StatusCode)
	}
	if resp.RequestID == "" {
		t.Error("request_id пуст — у федерации он приезжает ТОЛЬКО заголовком, " +
			"и пустота здесь значит, что клиент читает его из meta")
	}

	last, _ := s.Last()
	if last.Method != http.MethodPost || last.Path != "/users/upsert" {
		t.Errorf("позвали %s %s", last.Method, last.Path)
	}

	// ⚠⚠ Мёртвое обязательное поле JS-клиента. Дверь `external_id` не
	// принимает вовсе (zod отбрасывает посторонние ключи молча), и появись он
	// здесь — интегратор снова был бы уверен, что человека найдут по нему.
	var body map[string]json.RawMessage
	if err := json.Unmarshal(last.Body, &body); err != nil {
		t.Fatalf("тело запроса не разобралось: %v", err)
	}
	if _, present := body["external_id"]; present {
		t.Error("в теле upsert есть external_id — дверь его не читает, " +
			"и слать его значит повторять ошибку JS-клиента")
	}

	s.AssertNoIdempotencyHeader(t)
}

func TestUsersUpsertRefusesWithoutContact(t *testing.T) {
	s := federationtest.New(t)
	c := s.Client(t)

	_, _, err := c.Users.Upsert(ctx(), federation.UpsertUserInput{FirstName: "Пётр"})
	if !errors.Is(err, federation.ErrValidation) {
		t.Fatalf("ждали отказ проверки, получили %v", err)
	}
	// ⚠ Главное здесь — что до сети не дошло: отказ, потративший обращение,
	// ничем не лучше отказа двери.
	if n := len(s.Requests()); n != 0 {
		t.Errorf("клиент сходил в сеть %d раз, хотя вход негоден", n)
	}
}

// ── 2. users.Get ─────────────────────────────────────────────────────────────

func TestUsersGet(t *testing.T) {
	s := federationtest.New(t)
	c := s.Client(t)

	user, _, err := c.Users.Get(ctx(), "trid_conformance_01")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if user.ID != "trid_conformance_01" || user.FirstName != "Пётр" {
		t.Errorf("карточка разобрана не то: %+v", user)
	}
	last, _ := s.Last()
	if last.Path != "/users/trid_conformance_01" {
		t.Errorf("путь %s", last.Path)
	}
}

// ── 3. users.ListLeads ───────────────────────────────────────────────────────

func TestUsersListLeads(t *testing.T) {
	s := federationtest.New(t)
	c := s.Client(t)

	leads, _, err := c.Users.ListLeads(ctx(), "trid_conformance_01", federation.ListLeadsParams{
		Page:   federation.Page{Take: 50, Skip: 10},
		Status: "new",
	})
	if err != nil {
		t.Fatalf("ListLeads: %v", err)
	}
	if len(leads) != 2 || leads[0].ExternalID != "form-2026-0001" {
		t.Errorf("страница заявок разобрана не то: %+v", leads)
	}

	last, _ := s.Last()
	if last.Path != "/users/trid_conformance_01/leads" {
		t.Errorf("путь %s", last.Path)
	}
	// ⚠⚠ Именно `take`/`skip`, а не `limit`/`offset`: у CPA-двери они зовутся
	// иначе, и подстановка чужих имён дала бы МОЛЧА первую страницу вместо
	// запрошенной.
	if got := last.Query.Get("take"); got != "50" {
		t.Errorf("take=%q, ждали 50", got)
	}
	if got := last.Query.Get("skip"); got != "10" {
		t.Errorf("skip=%q, ждали 10", got)
	}
	if got := last.Query.Get("status"); got != "new" {
		t.Errorf("status=%q", got)
	}
	if last.Query.Has("limit") || last.Query.Has("offset") {
		t.Error("ушли limit/offset — это имена CPA-двери, федерация их не читает")
	}
}

// ── 4. users.Tree ────────────────────────────────────────────────────────────

func TestUsersTreeIdentityCannotBeConfused(t *testing.T) {
	s := federationtest.New(t)
	c := s.Client(t)

	tree, _, err := c.Users.Tree(ctx(), "trid_conformance_01", federation.TreeParams{Depth: 2})
	if err != nil {
		t.Fatalf("Tree: %v", err)
	}
	if got := s.Requests()[0].Query.Get("depth"); got != "2" {
		t.Errorf("depth=%q", got)
	}
	if len(tree.Levels) != 2 {
		t.Fatalf("колен %d, ждали 2", len(tree.Levels))
	}

	// Глубина 1 — суррогат, которым МОЖНО звать дверь.
	direct := tree.Levels[0].Nodes[0]
	if direct.Identity.Kind() != federation.IdentityDirect {
		t.Errorf("первое колено вида %q", direct.Identity.Kind())
	}
	if id, ok := direct.Identity.Direct(); !ok || id != "trid_direct_01" {
		t.Errorf("суррогат прямого партнёра: %q ok=%v", id, ok)
	}
	// ⚠⚠ И вот главное: у прямого партнёра НЕТ хеша, и попытка взять его
	// отвечает ложью, а не пустой строкой, которую легко не заметить.
	if _, ok := direct.Identity.Opaque(); ok {
		t.Error("у прямого партнёра нашёлся hashed_user_id — типу нечем было бы " +
			"помешать подставить одно вместо другого")
	}

	// Глубина 2 — необратимый хеш, которым нельзя ничего.
	deep := tree.Levels[1].Nodes[0]
	if deep.Identity.Kind() != federation.IdentityOpaque {
		t.Errorf("второе колено вида %q", deep.Identity.Kind())
	}
	if _, ok := deep.Identity.Direct(); ok {
		t.Error("хеш выдал себя за суррогат — им позвали бы дверь")
	}
	hash, ok := deep.Identity.Opaque()
	if !ok || !strings.HasPrefix(hash, "9f2a1c4e") {
		t.Errorf("хеш скрытого узла: %q ok=%v", hash, ok)
	}

	// ⚠⚠ Печать хеша. `fmt` — первое, что делает человек, разбираясь с
	// ответом, и полный хеш в журнале живёт дольше, чем нужно, будучи ни для
	// чего не годным.
	printed := deep.Identity.String()
	if printed == hash {
		t.Error("String() печатает хеш целиком — в журнале он осядет полностью")
	}
	if !strings.Contains(printed, "скрытый узел") {
		t.Errorf("String() = %q: не назвал, что это скрытый узел", printed)
	}
	// А суррогат печатается целиком: его и носят.
	if direct.Identity.String() != "trid_direct_01" {
		t.Errorf("суррогат печатается как %q", direct.Identity.String())
	}
}

// TestTreeNodeRefusesBothIdentities — контракт двери разошёлся с клиентом.
//
// ⚠⚠ Дверь по построению не смешивает формы: суррогат ставится на глубине 1,
// хеш — иначе. Приезд обеих означает, что что-то изменилось незаметно, и
// молча выбрать одну значило бы это спрятать — а выбрана была бы та, что
// стоит в коде первой, то есть решение приняла бы очерёдность строк.
func TestTreeNodeRefusesBothIdentities(t *testing.T) {
	var node federation.TreeNode
	err := json.Unmarshal([]byte(`{
	  "tenant_recca_id": "trid_1",
	  "hashed_user_id": "9f2a1c4e",
	  "display_name": "Анна", "revoked": false
	}`), &node)
	if err == nil {
		t.Fatalf("узел с ОБЕИМИ формами принят как %q — расхождение контракта спрятано",
			node.Identity.Kind())
	}

	// И обратный случай: ни одной формы — тоже отказ, а не «узел без личности».
	if err := json.Unmarshal([]byte(`{"display_name":"Аноним"}`), &node); err == nil {
		t.Fatal("узел без обеих форм принят — по чему его отличать от соседнего?")
	}
}

// ── 5. referralLinks.Create ──────────────────────────────────────────────────

func TestReferralLinksCreateIsIdempotentByPair(t *testing.T) {
	s := federationtest.New(t)
	c := s.Client(t)
	in := federation.CreateReferralLinkInput{OfferID: "offer_01"}

	first, resp1, err := c.ReferralLinks.Create(ctx(), "trid_conformance_01", in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if resp1.StatusCode != http.StatusCreated || first.Reused {
		t.Errorf("первая чеканка: статус %d reused=%v", resp1.StatusCode, first.Reused)
	}
	if first.SignedRef == "" || first.ShortURL == "" {
		t.Errorf("ссылка разобрана не то: %+v", first)
	}

	second, resp2, err := c.ReferralLinks.Create(ctx(), "trid_conformance_01", in)
	if err != nil {
		t.Fatalf("повтор: %v", err)
	}
	// ⚠⚠ Признак повтора живёт ТОЛЬКО в коде состояния: в теле его нет.
	// Потребитель, читающий одно тело, повтора не увидит вовсе — поэтому его
	// проставляет клиент.
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("повтор ответил %d, ждали 200", resp2.StatusCode)
	}
	if !second.Reused {
		t.Error("повтор не помечен Reused — потребитель решит, что выдана вторая ссылка")
	}
	if second.URL != first.URL {
		t.Errorf("повтор отдал ДРУГУЮ ссылку: %q против %q", second.URL, first.URL)
	}
	s.AssertNoIdempotencyHeader(t)
}

// ── 6. offers.List ───────────────────────────────────────────────────────────

func TestOffersList(t *testing.T) {
	s := federationtest.New(t)
	c := s.Client(t)

	offers, _, err := c.Offers.List(ctx(), federation.ListOffersParams{
		Page:   federation.Page{Take: 20},
		ShopID: "shop_conformance",
		Query:  "консульт",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(offers) != 2 {
		t.Fatalf("офферов %d, ждали 2", len(offers))
	}

	last, _ := s.Last()
	if last.Query.Get("q") != "консульт" || last.Query.Get("shop_id") != "shop_conformance" {
		t.Errorf("параметры отбора: %v", last.Query)
	}

	// ⚠⚠ `price = 0` при режиме «договорная» — это НЕ «0 ₽». Читать цену, не
	// прочитав режим, значит однажды напечатать «бесплатно» там, где написано
	// «цена договорная».
	if !offers[0].PriceKnown() {
		t.Error("у оффера с фиксированной ценой PriceKnown() ложно")
	}
	if offers[1].PriceKnown() {
		t.Error("у договорной цены PriceKnown() истинно — ноль уедет на экран как цена")
	}
}

// ── 7. leads.Create ──────────────────────────────────────────────────────────

func TestLeadsCreateIsIdempotentByExternalID(t *testing.T) {
	s := federationtest.New(t)
	c := s.Client(t)
	in := federation.CreateLeadInput{
		ReccaRef:   "SIGNED.REF.01",
		ExternalID: "form-2026-0001",
		Contact:    &federation.LeadContact{Name: "Клиент", Phone: "+79990000000"},
	}

	first, resp1, err := c.Leads.Create(ctx(), in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if first.Idempotent || resp1.StatusCode != http.StatusCreated {
		t.Errorf("первая заявка: idempotent=%v статус %d", first.Idempotent, resp1.StatusCode)
	}

	second, resp2, err := c.Leads.Create(ctx(), in)
	if err != nil {
		t.Fatalf("повтор: %v", err)
	}
	if !second.Idempotent || resp2.StatusCode != http.StatusOK {
		t.Errorf("повтор: idempotent=%v статус %d — ждали true и 200",
			second.Idempotent, resp2.StatusCode)
	}
	if second.LeadID != first.LeadID {
		t.Errorf("повтор завёл ВТОРУЮ заявку: %q против %q", second.LeadID, first.LeadID)
	}
	s.AssertNoIdempotencyHeader(t)
}

func TestLeadContactCarriesExtraFields(t *testing.T) {
	s := federationtest.New(t)
	c := s.Client(t)

	_, _, err := c.Leads.Create(ctx(), federation.CreateLeadInput{
		ReccaRef:   "SIGNED.REF.01",
		ExternalID: "form-extra",
		Contact: &federation.LeadContact{
			Phone: "+79990000000",
			Extra: map[string]any{"crm_id": "42", "phone": "+70000000000"},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	last, _ := s.Last()
	var body struct {
		Contact map[string]json.RawMessage `json:"contact"`
	}
	if err := json.Unmarshal(last.Body, &body); err != nil {
		t.Fatalf("тело не разобралось: %v", err)
	}
	if string(body.Contact["crm_id"]) != `"42"` {
		t.Errorf("посторонний ключ не доехал: %v", body.Contact)
	}
	// ⚠⚠ Своё поле СИЛЬНЕЕ Extra: без этого правила победил бы случайный
	// порядок обхода map, а телефон в заявке определяет, кому звонить.
	if string(body.Contact["phone"]) != `"+79990000000"` {
		t.Errorf("Extra перебил именованное поле: phone=%s", body.Contact["phone"])
	}
}

// ── 8. conversions.Register ──────────────────────────────────────────────────

func TestConversionsRegisterIsIdempotentByExternalID(t *testing.T) {
	s := federationtest.New(t)
	c := s.Client(t)
	in := federation.RegisterConversionInput{
		EventType:  "purchase",
		ExternalID: "order-2026-0001",
		AmountRub:  5000,
		ReccaRef:   "SIGNED.REF.01",
	}

	first, resp1, err := c.Conversions.Register(ctx(), in)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if first.Idempotent || resp1.StatusCode != http.StatusCreated {
		t.Errorf("первая конверсия: idempotent=%v статус %d", first.Idempotent, resp1.StatusCode)
	}
	if first.CascadeLegsCount != 2 || len(first.Legs) != 2 {
		t.Errorf("каскад разобран не то: %+v", first)
	}
	if first.AttributionSource != federation.AttributionTree {
		t.Errorf("путь привлечения %q", first.AttributionSource)
	}

	second, resp2, err := c.Conversions.Register(ctx(), in)
	if err != nil {
		t.Fatalf("повтор: %v", err)
	}
	if !second.Idempotent || resp2.StatusCode != http.StatusOK {
		t.Errorf("повтор: idempotent=%v статус %d — ждали true и 200",
			second.Idempotent, resp2.StatusCode)
	}
	if second.ConversionID != first.ConversionID {
		t.Errorf("повтор завёл ВТОРУЮ конверсию: %q против %q",
			second.ConversionID, first.ConversionID)
	}
	s.AssertNoIdempotencyHeader(t)
}

// TestConversionsRetryKeepsTheSameExternalID — мутация, которой держится вся
// идемпотентность федерации.
//
// ⚠⚠ Ключ живёт в ТЕЛЕ. Значит достаточно, чтобы повтор транспорта собрал тело
// заново с другим различителем, — и защита выключена, оставаясь с виду
// включённой: обе попытки успешны, обе ответили 201, потребитель видит успех,
// а в сети две конверсии и двойная оплата.
//
// Поэтому проверяется НЕ «повтор случился», а «повтор ушёл С ТЕМ ЖЕ
// external_id». Сломай это — и тест краснеет.
func TestConversionsRetryKeepsTheSameExternalID(t *testing.T) {
	s := federationtest.New(t)
	c, err := federation.New("rtk_test", &federation.ClientOptions{
		BaseURL:    s.URL(),
		MaxRetries: ptrInt(2),
	})
	if err != nil {
		t.Fatalf("клиент: %v", err)
	}

	// Отказ УДЕРЖАН: одноразовый снялся бы первой же попыткой, и повтора,
	// который мы измеряем, не случилось бы вовсе.
	s.HoldError(federation.CodeInternalError)

	_, _, err = c.Conversions.Register(ctx(), federation.RegisterConversionInput{
		EventType:  "purchase",
		ExternalID: "order-retry-0001",
		AmountRub:  5000,
		ReccaRef:   "SIGNED.REF.01",
	})
	if err == nil {
		t.Fatal("ждали отказ: стенд держал INTERNAL_ERROR")
	}

	reqs := s.Requests()
	if len(reqs) < 2 {
		t.Fatalf("обращений %d — повтора не было, и проверять нечего", len(reqs))
	}
	for i, r := range reqs {
		if got := bodyString(t, r.Body, "external_id"); got != "order-retry-0001" {
			t.Errorf("попытка %d ушла с external_id %q — ключ идемпотентности "+
				"пересобрался, и дверь приняла бы это как НОВУЮ конверсию", i+1, got)
		}
	}
}

// TestNonIdempotentPostIsNotRetried — обратная сторона того же решения.
func TestNonIdempotentPostIsNotRetried(t *testing.T) {
	s := federationtest.New(t)
	c, err := federation.New("rtk_test", &federation.ClientOptions{
		BaseURL:    s.URL(),
		MaxRetries: ptrInt(2),
	})
	if err != nil {
		t.Fatalf("клиент: %v", err)
	}
	s.HoldError(federation.CodeInternalError)

	// У просьбы раскрыть бейдж различителя нет: вторая попытка — второе
	// уведомление живому человеку.
	if _, _, err := c.Badges.RequestPublic(ctx(), "badge_01"); err == nil {
		t.Fatal("ждали отказ")
	}
	if n := len(s.Requests()); n != 1 {
		t.Errorf("обращений %d, ждали 1: операция без различителя повторяться не должна", n)
	}
}

// ── 9. conversions.Reverse ───────────────────────────────────────────────────

func TestConversionsReversePartialIsNotFull(t *testing.T) {
	s := federationtest.New(t)
	c := s.Client(t)

	res, resp, err := c.Conversions.Reverse(ctx(), "order-2026-0001",
		federation.ReverseConversionInput{Reason: "возврат покупателю"})
	if err != nil {
		t.Fatalf("Reverse: %v", err)
	}

	// ⚠⚠ Дверь отвечает 200 и при ЧАСТИЧНОЙ отмене. Прочитать 200 как «сняли
	// всё» — значит списать у себя больше, чем сняли у нас.
	if resp.StatusCode != http.StatusOK {
		t.Errorf("статус %d", resp.StatusCode)
	}
	if res.FullyReversed() {
		t.Error("частичная отмена посчиталась полной — по статусу 200 это и происходит")
	}
	if len(res.UnreversedLegs) != 1 || res.UnreversedLegs[0].UnreversedReason != "ALREADY_PAID" {
		t.Errorf("неотменённые ноги разобраны не то: %+v", res.UnreversedLegs)
	}
	if res.ReversedAmountRub != 500 {
		t.Errorf("сняли %v, ждали 500", res.ReversedAmountRub)
	}
}

func TestConversionsReverseEscapesExternalID(t *testing.T) {
	s := federationtest.New(t)
	c := s.Client(t)

	// ⚠⚠ external_id — строка без ограничения алфавита: слеш и вопрос в ней
	// законны. Подставь её в путь как есть — и «order/17?x=1» станет другим
	// путём с параметрами, а дверь ответит про другую конверсию либо 404.
	_, _, _ = c.Conversions.Reverse(ctx(), "order/17?x=1",
		federation.ReverseConversionInput{Reason: "проверка экранирования"})

	last, ok := s.Last()
	if !ok {
		t.Fatal("обращений нет")
	}
	if last.Query.Has("x") {
		t.Error("часть external_id уехала в строку запроса — путь не экранирован")
	}
	// ⚠ Сверяем СЫРОЙ путь: `net/http` раскрывает проценты до обработчика, и
	// на декодированном «order%2F17» неотличимо от «order/17» — проверка была
	// бы зелёной ровно на той дыре, которую ищет.
	if !strings.Contains(last.EscapedPath, "order%2F17%3Fx=1") {
		t.Errorf("сырой путь %q: external_id не экранирован", last.EscapedPath)
	}
	if last.Path != "/conversions/order/17?x=1/reverse" {
		t.Errorf("декодированный путь %q — стенд собрал его не из того сегмента", last.Path)
	}
}

func TestConversionsReverseRefusesWithoutReason(t *testing.T) {
	s := federationtest.New(t)
	c := s.Client(t)

	_, _, err := c.Conversions.Reverse(ctx(), "order-1", federation.ReverseConversionInput{})
	if !errors.Is(err, federation.ErrValidation) {
		t.Fatalf("ждали отказ проверки, получили %v", err)
	}
	if n := len(s.Requests()); n != 0 {
		t.Errorf("сходили в сеть %d раз: отмена денег без причины не разбирается потом никем", n)
	}
}

// ── 10. promos.Balance ───────────────────────────────────────────────────────

// TestPromosBalanceSeparatesLedgerFromWallet — то, ради чего ST-6 шёл первым.
func TestPromosBalanceSeparatesLedgerFromWallet(t *testing.T) {
	s := federationtest.New(t)
	c := s.Client(t)

	b, _, err := c.Promos.Balance(ctx(), "trid_conformance_01")
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if len(b.CurrencyBreakdown) != 1 {
		t.Fatalf("валют %d", len(b.CurrencyBreakdown))
	}
	rub := b.CurrencyBreakdown[0]

	// Леджер: начислено минус отменено. Вывод его НЕ уменьшает.
	if rub.LifetimeAccrued != 1500 || rub.LifetimeReversed != 300 || rub.LifetimeNet != 1200 {
		t.Errorf("леджер разобран не то: %+v", rub)
	}
	// Кошелёк: сколько можно потратить СЕЙЧАС.
	if rub.WalletCleared == nil || *rub.WalletCleared != 900 {
		t.Errorf("кошелёк разобран не то: %v", rub.WalletCleared)
	}
	if rub.WalletPending == nil || *rub.WalletPending != 400 {
		t.Errorf("дозревающее разобрано не то: %v", rub.WalletPending)
	}

	// ⚠⚠ И главное: числа РАЗНЫЕ, и подстановка одного вместо другого видна.
	if rub.LifetimeNet == *rub.WalletCleared {
		t.Fatal("фикстура сделала леджер равным кошельку — на таких данных " +
			"путаница полей неотличима от правильного разбора")
	}

	spendable, known := b.Spendable()
	if !known || spendable != 900 {
		t.Errorf("Spendable() = %v, %v — ждали 900, true", spendable, known)
	}
	if b.TotalLifetimeNetRUB != 1200 {
		t.Errorf("total_available_rub = %v", b.TotalLifetimeNetRUB)
	}
}

// TestPromosBalanceUnknownWalletIsNotZero — «не знаю» не превращается в «нет».
func TestPromosBalanceUnknownWalletIsNotZero(t *testing.T) {
	var b federation.Balance
	raw := `{
	  "currency_breakdown": [
	    {"currency":"RUB","accrued":1500,"reversed":300,"available":1200,
	     "wallet_cleared":null,"wallet_pending":null}
	  ],
	  "total_available_rub": 1200,
	  "total_wallet_cleared_rub": null
	}`
	if err := json.Unmarshal([]byte(raw), &b); err != nil {
		t.Fatalf("разбор: %v", err)
	}

	if b.CurrencyBreakdown[0].WalletCleared != nil {
		t.Error("null кошелька разобрался НЕ в nil — «не знаю» стало числом")
	}
	spendable, known := b.Spendable()
	if known {
		t.Error("Spendable() отчитался, что знает, — а дверь сказала обратное")
	}
	if spendable != 0 {
		t.Errorf("Spendable() = %v при неизвестном кошельке", spendable)
	}
	// ⚠ Леджер при этом известен: неизвестность кошелька его не отменяет.
	if b.TotalLifetimeNetRUB != 1200 {
		t.Errorf("леджер потерялся: %v", b.TotalLifetimeNetRUB)
	}
}

// ── 11. badges.RequestPublic ─────────────────────────────────────────────────

func TestBadgesRequestPublicDoesNotChangeVisibility(t *testing.T) {
	s := federationtest.New(t)
	c := s.Client(t)

	res, _, err := c.Badges.RequestPublic(ctx(), "badge_01")
	if err != nil {
		t.Fatalf("RequestPublic: %v", err)
	}
	if !res.Requested {
		t.Error("просьба не помечена отправленной")
	}
	// ⚠⚠ Дверь ПРОСИТ, а не меняет. Прочитать Requested как «стало публичным»
	// — значит показать у себя бейдж, которого человек не раскрывал.
	if res.Badge.EffectiveVisibility != "tenant_only" {
		t.Errorf("действующая видимость %q — просьба не должна её менять",
			res.Badge.EffectiveVisibility)
	}
	last, _ := s.Last()
	if last.Path != "/badges/badge_01/request-public" {
		t.Errorf("путь %s", last.Path)
	}
}

// ── отказы ───────────────────────────────────────────────────────────────────

// TestNotMemberSynonymsAnswerTheSameQuestion — два живых кода на один факт.
func TestNotMemberSynonymsAnswerTheSameQuestion(t *testing.T) {
	for _, code := range []federation.ErrorCode{
		federation.CodeNotAMember,
		federation.CodeUserNotMemberOfTenant,
	} {
		t.Run(string(code), func(t *testing.T) {
			s := federationtest.New(t)
			c := s.Client(t)
			s.PrepareError(code)

			_, _, err := c.Promos.Balance(ctx(), "trid_conformance_01")
			if err == nil {
				t.Fatal("ждали отказ")
			}
			// ⚠⚠ Ветвление по ОДНОМУ из кодов зелено ровно до той двери,
			// которая отвечает вторым. Спрашивать надо про факт.
			if !federation.IsNotMember(err) {
				t.Errorf("IsNotMember ложно на коде %s — потребитель, "+
					"ветвящийся по факту, этот отказ пропустит", code)
			}
			if !errors.Is(err, federation.ErrNotMember) {
				t.Errorf("errors.Is(ErrNotMember) ложно на коде %s", code)
			}

			var apiErr *federation.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("отказ не APIError: %v", err)
			}
			if apiErr.Code != code {
				t.Errorf("код %s, ждали %s", apiErr.Code, code)
			}
			if apiErr.RequestID == "" {
				t.Error("у отказа нет request_id — а он приезжает заголовком, " +
					"и блока meta у отказа федерации нет вовсе")
			}
		})
	}
}

func TestCodesCoverTheRegistry(t *testing.T) {
	// ⚠ Сверка с числом кодов реестра дверей: 38 (5 рамы + 4 общих с CPA +
	// 29 своих). Меньше — клиент не знает кода, который дверь присылает.
	const want = 38
	// MALFORMED_RESPONSE ставит сам клиент, в реестре дверей его нет.
	got := 0
	for _, code := range federation.Codes() {
		if code != federation.CodeMalformedResponse {
			got++
		}
	}
	if got != want {
		t.Errorf("клиент знает %d кодов федерации, реестр дверей объявляет %d", got, want)
	}
}

// ── подпорки ─────────────────────────────────────────────────────────────────

func ptrInt(v int) *int { return &v }

func bodyString(t *testing.T, body []byte, field string) string {
	t.Helper()
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		t.Fatalf("тело не разобралось: %v", err)
	}
	var s string
	if err := json.Unmarshal(obj[field], &s); err != nil {
		t.Fatalf("поле %s не строка: %v", field, err)
	}
	return s
}
