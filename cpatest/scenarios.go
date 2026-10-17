package cpatest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"go.recca.ru/sdk/cpa"
	"go.recca.ru/sdk/cpatypes"
)

// ScenarioOptions — сценарии приёмки из дополнения к Приложению № 1 договора
// (раздел 6). Каждый сценарий идёт на СВОЁМ оффере, потому что доказывает
// свойство оффера: цель «регистрация бренда», ставку конкретной цели, долю
// revshare. Сценарий с пустым оффером пропускается и называется в журнале
// пропущенным — молча он не выпадает.
//
// ⚠ Ожидаемые суммы набор ВЫВОДИТ из условий оффера, прочитанных у той же
// двери, а не берёт из договора: одна и та же библиотека приёмки годится для
// любой сети, а цифры договора (15 % / 5 %) проверяются тем, что в протокол
// попадают вместе с ответом двери.
type ScenarioOptions struct {
	// BrandOfferID и BrandGoal — сценарий 1: оффер с целью «регистрация бренда».
	BrandOfferID string
	BrandGoal    string
	// AgentPercent — сценарий 2: ставка рекрутёра, с которой набор его заводит.
	// Пусто — "10". Рекрутёр и вебмастер заводятся на оффере RevshareOfferID.
	AgentPercent string
	// GoalOfferID и Goal — сценарий 3: оффер с целями, у каждой своя ставка.
	GoalOfferID string
	Goal        string
	// RevshareOfferID — сценарий 4 (и площадка сценария 2).
	RevshareOfferID string
	// MultiOfferID, MultiGoal и MultiPlainGoal — сценарий 6 (WV-0827): оффер,
	// где ОДНА цель принимает несколько конверсий с одного клика (флаг цели
	// включает оператор), а ДРУГАЯ — нет. Пара «разрешение/запрет» обязана быть
	// на одном оффере: запрет на чужих условиях доказывал бы чужое свойство.
	MultiOfferID   string
	MultiGoal      string
	MultiPlainGoal string
	// SumRUB — сумма заказа для процентных ставок. Пусто — 10000: сумма, при
	// которой доли договора (15 %, 5 %, 10 %) дают целые рубли.
	SumRUB cpa.RUB
}

func (s ScenarioOptions) sum() cpa.RUB {
	if s.SumRUB > 0 {
		return s.SumRUB
	}
	return 10000
}

func (s ScenarioOptions) agentPercent() string {
	if s.AgentPercent != "" {
		return s.AgentPercent
	}
	return "10"
}

// runScenarios гоняет сценарии договора. Каждый независим: падение одного не
// прячет остальные.
func runScenarios(ctx context.Context, t TB, c *cpa.Client, opts ConformanceOptions) {
	s := *opts.Scenarios
	stamp := fmt.Sprint(opts.now().Unix())

	type scenario struct {
		name    string
		offerID string
		run     func() error
	}
	all := []scenario{
		{"1 (бренды и блогеры)", s.BrandOfferID, func() error { return scenarioBrand(ctx, t, c, opts, stamp) }},
		{"2 (скаут)", s.RevshareOfferID, func() error { return scenarioScout(ctx, t, c, opts, stamp) }},
		{"3 (целевое действие)", s.GoalOfferID, func() error { return scenarioGoal(ctx, t, c, opts, stamp) }},
		{"4 (revshare)", s.RevshareOfferID, func() error { return scenarioRevshare(ctx, t, c, opts, stamp) }},
		{"6 (несколько конверсий с клика)", s.MultiOfferID, func() error { return scenarioMulti(ctx, t, c, opts, stamp) }},
	}
	for _, sc := range all {
		if sc.offerID == "" {
			t.Logf("сценарий договора %s пропущен: оффер не задан", sc.name)
			continue
		}
		if err := sc.run(); err != nil {
			t.Errorf("сценарий договора %s: %v", sc.name, err)
		}
	}
}

// ── общие шаги ───────────────────────────────────────────────────────────────

// converted — итог сквозной операции «ссылка → клик → конверсия → карточка».
type converted struct {
	card      *cpa.ConversionDetail
	requestID string // request_id приёма конверсии — главный для протокола
}

// convert проводит сквозную операцию и пишет request_id каждого шага: протокол
// приёмки по договору обязан содержать идентификатор каждого запроса.
func convert(ctx context.Context, t TB, c *cpa.Client, step string, partner cpa.PartnerExternalID,
	offerID string, goal *string, sum *cpa.RUB, externalID, clickKey string) (*converted, error) {
	link, resp, err := c.Links.Create(ctx, cpa.CreateLinkParams{PartnerExternalID: partner, OfferID: offerID})
	if err != nil {
		return nil, fmt.Errorf("ссылка: %w", err)
	}
	t.Logf("  %s · ссылка %s — request_id %s", step, link.Code, resp.RequestID)

	click, resp, err := c.Clicks.Create(ctx, cpa.CreateClickParams{
		PartnerExternalID: partner, OfferID: offerID, ClientClickID: clickKey, Country: "RU",
	})
	if err != nil {
		return nil, fmt.Errorf("переход: %w", err)
	}
	t.Logf("  %s · переход — request_id %s", step, resp.RequestID)
	if click.Status == cpa.ClickTrafficback || click.ClickID == nil {
		return nil, fmt.Errorf("переход отбит (%s) — сценарий дальше не идёт",
			valueOr(click.Reason, "click_id не выдан"))
	}

	conversion, resp, err := c.Conversions.Create(ctx, cpa.CreateConversionParams{
		ClickID: *click.ClickID, ExternalID: externalID, Status: "approved", Goal: goal, SumRUB: sum,
	})
	if err != nil {
		return nil, fmt.Errorf("конверсия: %w", err)
	}
	t.Logf("  %s · конверсия %s — request_id %s", step, conversion.ExternalID, resp.RequestID)
	created := resp.RequestID

	card, resp, err := c.Conversions.Get(ctx, cpa.GetConversionParams{ExternalID: externalID, Goal: goal})
	if err != nil {
		return nil, fmt.Errorf("карточка конверсии (метод 13): %w", err)
	}
	t.Logf("  %s · карточка (метод 13): выручка %s = выплата %s + маржа %s — request_id %s",
		step, card.RevenueRUB, card.PayoutRUB, card.MarginRUB, resp.RequestID)
	if err := checkTriple(card.RevenueRUB, card.PayoutRUB, card.MarginRUB); err != nil {
		return nil, err
	}
	return &converted{card: card, requestID: created}, nil
}

func upsertWebmaster(ctx context.Context, t TB, c *cpa.Client, step string, id cpa.PartnerExternalID) error {
	_, resp, err := c.Partners.Upsert(ctx, cpa.UpsertPartnerParams{
		ExternalID: id, Role: cpa.PartnerWebmaster, PartnerState: cpa.PartnerStateActive,
	})
	if err != nil {
		return fmt.Errorf("партнёр %s: %w", id, err)
	}
	t.Logf("  %s · партнёр %s (метод 2) — request_id %s", step, id, resp.RequestID)
	return nil
}

// ── сценарий 1: бренды и блогеры ─────────────────────────────────────────────

func scenarioBrand(ctx context.Context, t TB, c *cpa.Client, opts ConformanceOptions, stamp string) error {
	s := opts.Scenarios
	if s.BrandGoal == "" {
		return errors.New("не задана цель «регистрация бренда» (BrandGoal)")
	}
	partner := cpa.PartnerExternalID(opts.prefix() + "s1-referrer")
	if err := upsertWebmaster(ctx, t, c, "С1", partner); err != nil {
		return err
	}
	pay, rev, err := goalRates(ctx, c, s.BrandOfferID, s.BrandGoal)
	if err != nil {
		return err
	}
	sum := s.sum()
	got, err := convert(ctx, t, c, "С1", partner, s.BrandOfferID, &s.BrandGoal, &sum,
		opts.prefix()+"s1-order-"+stamp, opts.prefix()+"s1-click-"+stamp)
	if err != nil {
		return err
	}
	if err := checkAmounts(got.card, pay, rev, sum); err != nil {
		return err
	}
	if got.card.PartnerExternalID == nil || *got.card.PartnerExternalID != partner {
		return fmt.Errorf("приведший указан неверно: %v, ждали %s", ptrString(got.card.PartnerExternalID), partner)
	}
	t.Logf("сценарий договора 1 пройден: цель %s, приведший %s, выручка %s = выплата %s + маржа %s",
		got.card.Goal, partner, got.card.RevenueRUB, got.card.PayoutRUB, got.card.MarginRUB)
	return nil
}

// ── сценарий 2: скаут ────────────────────────────────────────────────────────

func scenarioScout(ctx context.Context, t TB, c *cpa.Client, opts ConformanceOptions, stamp string) error {
	s := opts.Scenarios
	self, resp, err := c.Self.Get(ctx)
	if err != nil {
		return fmt.Errorf("база ноги сети (метод 1): %w", err)
	}
	t.Logf("  С2 · база ноги рекрутёра у сети: %s — request_id %s", self.AgentBase, resp.RequestID)

	// ⚠ Рекрутёр свой на каждый прогон: ставку действующего агента дверь на
	// повторе не меняет (400), и прогон с другой AgentPercent упёрся бы в отказ.
	agent := cpa.PartnerExternalID(opts.prefix() + "s2-agent-" + stamp)
	percent := json.Number(s.agentPercent())
	_, resp, err = c.Partners.Upsert(ctx, cpa.UpsertPartnerParams{
		ExternalID: agent, Role: cpa.PartnerAgent, AgentPercent: &percent,
	})
	if err != nil {
		return fmt.Errorf("рекрутёр: %w", err)
	}
	t.Logf("  С2 · рекрутёр %s, ставка %s %% (метод 2) — request_id %s", agent, percent, resp.RequestID)

	web := cpa.PartnerExternalID(opts.prefix() + "s2-webmaster-" + stamp)
	_, resp, err = c.Partners.Upsert(ctx, cpa.UpsertPartnerParams{
		ExternalID: web, Role: cpa.PartnerWebmaster, RecruiterExternalID: agent,
	})
	if err != nil {
		return fmt.Errorf("вебмастер с рекрутёром: %w", err)
	}
	t.Logf("  С2 · вебмастер %s с рекрутёром %s (метод 2) — request_id %s", web, agent, resp.RequestID)

	sum := s.sum()
	got, err := convert(ctx, t, c, "С2", web, s.RevshareOfferID, nil, &sum,
		opts.prefix()+"s2-order-"+stamp, opts.prefix()+"s2-click-"+stamp)
	if err != nil {
		return err
	}
	if err := checkAgentLeg(got.card, agent, self.AgentBase, percent); err != nil {
		return err
	}
	leg := got.card.AgentLeg
	t.Logf("сценарий договора 2 пройден: нога рекрутёра %s ₽ = %s %% от %s (%s)",
		leg.AmountRUB, percent, leg.Base, legBaseAmount(got.card, leg.Base))
	return nil
}

// ── сценарий 3: целевое действие ─────────────────────────────────────────────

func scenarioGoal(ctx context.Context, t TB, c *cpa.Client, opts ConformanceOptions, stamp string) error {
	s := opts.Scenarios
	if s.Goal == "" {
		return errors.New("не задана цель (Goal)")
	}
	offer, _, err := c.Offers.GetForPartner(ctx, s.GoalOfferID)
	if err != nil {
		return fmt.Errorf("оффер %s: %w", s.GoalOfferID, err)
	}
	pay, rev, err := goalRates(ctx, c, s.GoalOfferID, s.Goal)
	if err != nil {
		return err
	}
	sum := s.sum()
	// ⚠ Ставка цели обязана ОТЛИЧАТЬСЯ от ставки оффера — иначе «применена
	// ставка цели» неотличимо от «применена ставка оффера», и сценарий был бы
	// зелёным по неправильной причине.
	wantGoal, err := expectedRUB(pay, sum)
	if err != nil {
		return err
	}
	if wantOffer, err := expectedRUB(offer.PayoutRate, sum); err == nil && wantOffer == wantGoal {
		return fmt.Errorf("ставка цели %s совпадает со ставкой оффера (%s ₽) — сценарий не различает их; нужен оффер, где они разные",
			s.Goal, wantGoal)
	}

	partner := cpa.PartnerExternalID(opts.prefix() + "s3-webmaster")
	if err := upsertWebmaster(ctx, t, c, "С3", partner); err != nil {
		return err
	}
	got, err := convert(ctx, t, c, "С3", partner, s.GoalOfferID, &s.Goal, &sum,
		opts.prefix()+"s3-order-"+stamp, opts.prefix()+"s3-click-"+stamp)
	if err != nil {
		return err
	}
	if got.card.Goal != s.Goal {
		return fmt.Errorf("конверсия записана на цель %q, отправляли %q", got.card.Goal, s.Goal)
	}
	if err := checkAmounts(got.card, pay, rev, sum); err != nil {
		return err
	}
	if err := checkHold(got.card, goalHoldDays(offer, s.Goal), opts.now()); err != nil {
		return err
	}
	t.Logf("сценарий договора 3 пройден: цель %s, выплата %s ₽ по ставке цели, холд до %s. "+
		"⚠ Смену статуса по истечении холда и уведомление набор в том же прогоне не измеряет — "+
		"их доказывает приёмник вебхуков после холда", got.card.Goal, got.card.PayoutRUB,
		got.card.HoldUntil.Format(time.RFC3339))
	return nil
}

// ── сценарий 4: revshare ─────────────────────────────────────────────────────

func scenarioRevshare(ctx context.Context, t TB, c *cpa.Client, opts ConformanceOptions, stamp string) error {
	s := opts.Scenarios
	partnerView, _, err := c.Offers.GetForPartner(ctx, s.RevshareOfferID)
	if err != nil {
		return fmt.Errorf("оффер %s: %w", s.RevshareOfferID, err)
	}
	// Рекламодатель — из партнёрской проекции ТОГО ЖЕ оффера: с 02.10 дверь
	// требует advertiser_id и на карточке (WV-0828 Д2R1-07), а приёмке
	// достаточно оффера из опций — лишнего env не нужно.
	advertiserView, _, err := c.Offers.GetForAdvertiser(ctx, s.RevshareOfferID, partnerView.AdvertiserID)
	if err != nil {
		return fmt.Errorf("оффер %s глазами рекламодателя: %w", s.RevshareOfferID, err)
	}
	if partnerView.PayoutRate.Kind != cpa.RatePercent || advertiserView.RevenueRate.Kind != cpa.RatePercent {
		return fmt.Errorf("оффер %s не revshare: ставки не процентные", s.RevshareOfferID)
	}
	partner := cpa.PartnerExternalID(opts.prefix() + "s4-webmaster")
	if err := upsertWebmaster(ctx, t, c, "С4", partner); err != nil {
		return err
	}
	sum := s.sum()
	got, err := convert(ctx, t, c, "С4", partner, s.RevshareOfferID, nil, &sum,
		opts.prefix()+"s4-order-"+stamp, opts.prefix()+"s4-click-"+stamp)
	if err != nil {
		return err
	}
	if err := checkAmounts(got.card, partnerView.PayoutRate, advertiserView.RevenueRate, sum); err != nil {
		return err
	}
	t.Logf("сценарий договора 4 пройден: сумма заказа %s ₽ → выручка %s (%s %%), партнёру %s (%s %%), оператору %s",
		sum, got.card.RevenueRUB, *advertiserView.RevenueRate.ValuePercent,
		got.card.PayoutRUB, *partnerView.PayoutRate.ValuePercent, got.card.MarginRUB)
	return nil
}

// ── сценарий 6: несколько конверсий с одного клика ──────────────────────────

// scenarioMulti — WV-0827: пара «клик + цель» платится один раз, если у цели
// не включено «несколько конверсий с одного клика». Флаг цель несёт в ОБЕИХ
// проекциях оффера, и обе обязаны его донести: разойдись проекции, интегратор
// читал бы флаг одной стороны и решал по нему, слать ли второй заказ той же
// цели.
func scenarioMulti(ctx context.Context, t TB, c *cpa.Client, opts ConformanceOptions, stamp string) error {
	s := opts.Scenarios
	if s.MultiGoal == "" || s.MultiPlainGoal == "" {
		return errors.New("не заданы цели сценария: MultiGoal (с флагом) и MultiPlainGoal (без)")
	}

	partnerView, _, err := c.Offers.GetForPartner(ctx, s.MultiOfferID)
	if err != nil {
		return fmt.Errorf("оффер %s: %w", s.MultiOfferID, err)
	}
	advertiserView, _, err := c.Offers.GetForAdvertiser(ctx, s.MultiOfferID, partnerView.AdvertiserID)
	if err != nil {
		return fmt.Errorf("оффер %s глазами рекламодателя: %w", s.MultiOfferID, err)
	}
	multiPartner, multiAdv := false, false
	for _, g := range partnerView.Goals {
		if g.Code == s.MultiGoal && g.MultiplePerClick {
			multiPartner = true
		}
	}
	for _, g := range advertiserView.Goals {
		if g.Code == s.MultiGoal && g.MultiplePerClick {
			multiAdv = true
		}
	}
	if !multiPartner || !multiAdv {
		return fmt.Errorf("у цели %q флаг multiple_per_click не включён (партнёрская проекция %v, рекламодательская %v) — его ставит оператор сети; без флага сценарий доказывал бы отсутствие настройки, а не поведение",
			s.MultiGoal, multiPartner, multiAdv)
	}
	for _, g := range partnerView.Goals {
		if g.Code == s.MultiPlainGoal && g.MultiplePerClick {
			return fmt.Errorf("у цели %q флаг включён — на ветку запрета нужна цель БЕЗ флага, иначе запрет был бы зелёным по чужой причине",
				s.MultiPlainGoal)
		}
	}

	pay, rev, err := goalRates(ctx, c, s.MultiOfferID, s.MultiGoal)
	if err != nil {
		return err
	}
	hold := goalHoldDays(partnerView, s.MultiGoal)
	sum := s.sum()
	goal := s.MultiGoal

	partner := cpa.PartnerExternalID(opts.prefix() + "s6-webmaster")
	if err := upsertWebmaster(ctx, t, c, "С6", partner); err != nil {
		return err
	}

	// ── нога «разрешено»: ОДИН клик, ДВА заказа одной целью ────────────────
	// Каждый платится СВОЕЙ конверсией — со своей выплатой и своим холдом.
	click, err := s6Click(ctx, t, c, partner, s.MultiOfferID, opts.prefix()+"s6-click-"+stamp)
	if err != nil {
		return err
	}
	extA, extB := opts.prefix()+"s6-a-"+stamp, opts.prefix()+"s6-b-"+stamp

	first, firstResp, err := c.Conversions.Create(ctx, cpa.CreateConversionParams{
		ClickID: click, ExternalID: extA, Status: "approved", Goal: &goal, SumRUB: &sum,
	})
	if err != nil {
		return fmt.Errorf("первая конверсия цели с флагом: %w", err)
	}
	t.Logf("  С6 · конверсия %s — request_id %s", first.ExternalID, firstResp.RequestID)
	if firstResp.StatusCode != http.StatusCreated {
		return fmt.Errorf("первая конверсия ответила %d, ждали ровно 201", firstResp.StatusCode)
	}
	if err := checkAmounts(first, pay, rev, sum); err != nil {
		return err
	}
	if err := checkHold(first, hold, opts.now()); err != nil {
		return err
	}

	second, secondResp, err := c.Conversions.Create(ctx, cpa.CreateConversionParams{
		ClickID: click, ExternalID: extB, Status: "approved", Goal: &goal, SumRUB: &sum,
	})
	if err != nil {
		return fmt.Errorf("второй заказ %s той же цели с того же клика отбит, а флаг у цели включён: %w", extB, err)
	}
	t.Logf("  С6 · конверсия %s — request_id %s", second.ExternalID, secondResp.RequestID)
	if secondResp.StatusCode != http.StatusCreated {
		return fmt.Errorf("вторая конверсия ответила %d, ждали ровно 201", secondResp.StatusCode)
	}
	if second.ExternalID != extB {
		return fmt.Errorf("дверь вернула конверсию %s вместо %s", second.ExternalID, extB)
	}
	if err := checkAmounts(second, pay, rev, sum); err != nil {
		return err
	}
	if err := checkHold(second, hold, opts.now()); err != nil {
		return err
	}

	// Повтор ПЕРВОГО external_id идемпотентен: у цели с флагом «второй заказ»
	// и «повтор первого» — разные события, и повтор не создаёт третью ногу.
	repeat, repeatResp, err := c.Conversions.Create(ctx, cpa.CreateConversionParams{
		ClickID: click, ExternalID: extA, Status: "approved", Goal: &goal, SumRUB: &sum,
	})
	if err != nil {
		return fmt.Errorf("идемпотентный повтор первого заказа отбит: %w", err)
	}
	if repeat.ExternalID != first.ExternalID {
		return fmt.Errorf("повтор вернул другую конверсию: %s вместо %s", repeat.ExternalID, first.ExternalID)
	}
	if repeatResp.StatusCode != http.StatusOK {
		return fmt.Errorf("повтор ответил %d, ждали 200 — конверсия уже была", repeatResp.StatusCode)
	}

	// Отмена одной гасит ТОЛЬКО её: вторая жива — и деньгами, и статусом.
	cancelled, cancelResp, err := c.Conversions.Cancel(ctx, cpa.CancelConversionParams{
		ExternalID: extA, Goal: &goal, Reason: "сценарий 6: отмена первой из двух",
	})
	if err != nil {
		return fmt.Errorf("отмена первой из двух: %w", err)
	}
	t.Logf("  С6 · отмена %s — request_id %s", extA, cancelResp.RequestID)
	if cancelled.Status != cpa.ConversionDeclined {
		return fmt.Errorf("отмена оставила статус %q, ждали declined", cancelled.Status)
	}
	cardB, _, err := c.Conversions.Get(ctx, cpa.GetConversionParams{ExternalID: extB, Goal: &goal})
	if err != nil {
		return fmt.Errorf("карточка второй конверсии после отмены первой: %w", err)
	}
	if cardB.Status == cpa.ConversionDeclined {
		return errors.New("отмена первой погасила и вторую — у цели с флагом каждая конверсия своя")
	}
	if err := checkAmounts(cardB, pay, rev, sum); err != nil {
		return err
	}

	// ── нога «запрет»: та же пара у цели БЕЗ флага ──────────────────────────
	plainGoal := s.MultiPlainGoal
	plainClick, err := s6Click(ctx, t, c, partner, s.MultiOfferID, opts.prefix()+"s6-plain-click-"+stamp)
	if err != nil {
		return err
	}
	extC, extD := opts.prefix()+"s6-c-"+stamp, opts.prefix()+"s6-d-"+stamp

	if _, _, err := c.Conversions.Create(ctx, cpa.CreateConversionParams{
		ClickID: plainClick, ExternalID: extC, Status: "approved", Goal: &plainGoal, SumRUB: &sum,
	}); err != nil {
		return fmt.Errorf("первая конверсия цели без флага: %w", err)
	}
	_, _, err = c.Conversions.Create(ctx, cpa.CreateConversionParams{
		ClickID: plainClick, ExternalID: extD, Status: "approved", Goal: &plainGoal, SumRUB: &sum,
	})
	if err == nil {
		return fmt.Errorf("второй заказ %s на цели %q без флага прошёл — пара «клик + цель» платится один раз",
			extD, plainGoal)
	}
	if !errors.Is(err, cpa.ErrAlreadyConverted) {
		return fmt.Errorf("ждали ALREADY_CONVERTED, получили: %w", err)
	}
	var apiErr *cpa.APIError
	if !errors.As(err, &apiErr) {
		return fmt.Errorf("отказ не разобран как ответ двери: %w", err)
	}
	if apiErr.HTTPStatus != http.StatusConflict {
		return fmt.Errorf("ALREADY_CONVERTED приехал со статусом %d, ждали 409", apiErr.HTTPStatus)
	}
	// Отказ называет заказ, занявший пару (WV-0827): без номера интегратор не
	// отличил бы «мой повтор» от «пару занял другой заказ».
	if !strings.Contains(apiErr.Message, extC) {
		return fmt.Errorf("отказ не называет занявший пару заказ %s: %q", extC, apiErr.Message)
	}

	t.Logf("сценарий договора 6 пройден: цель %s (флаг) — %s и %s с одного клика, каждая со своей выплатой %s ₽ и холдом; повтор %s идемпотентен; после отмены %s конверсия %s жива (%s); цель %s без флага — второй заказ 409 ALREADY_CONVERTED с номером занявшего",
		s.MultiGoal, extA, extB, first.PayoutRUB, extA, extA, extB, cardB.Status, s.MultiPlainGoal)
	return nil
}

// s6Click — ссылка и клик с уникальным на прогон ключом. Сценарий меряет ПАРУ
// «клик + цель», и клик обязан быть свежим: повторный ключ вернул бы клик
// прошлого прогона с его занятыми парами. Идентификатор — именованный
// cpatypes.ClickID на всём пути сценария: он затем и именованный, чтобы id
// клика не путался с другими строками.
func s6Click(ctx context.Context, t TB, c *cpa.Client, partner cpa.PartnerExternalID, offerID, clickKey string) (cpatypes.ClickID, error) {
	link, resp, err := c.Links.Create(ctx, cpa.CreateLinkParams{PartnerExternalID: partner, OfferID: offerID})
	if err != nil {
		return "", fmt.Errorf("ссылка: %w", err)
	}
	t.Logf("  С6 · ссылка %s — request_id %s", link.Code, resp.RequestID)

	click, resp, err := c.Clicks.Create(ctx, cpa.CreateClickParams{
		PartnerExternalID: partner, OfferID: offerID, ClientClickID: clickKey, Country: "RU",
	})
	if err != nil {
		return "", fmt.Errorf("переход: %w", err)
	}
	if click.Status == cpa.ClickTrafficback || click.ClickID == nil {
		return "", fmt.Errorf("переход отбит (%s) — сценарий дальше не идёт",
			valueOr(click.Reason, "click_id не выдан"))
	}
	t.Logf("  С6 · переход — request_id %s", resp.RequestID)
	return *click.ClickID, nil
}

// ── чистые проверки (их доказывают юниты и мутации) ─────────────────────────

// expectedRUB — сколько по ставке причитается с суммы заказа. Процентная ставка
// обязана давать ЦЕЛЫЕ рубли: округление — политика двери, и набор, угадывающий
// его, был бы зелёным или красным по чужой причине. Нецелое — ошибка выбора
// суммы, а не расчёта.
func expectedRUB(r cpa.Rate, sum cpa.RUB) (cpa.RUB, error) {
	switch r.Kind {
	case cpa.RateFixed:
		if r.ValueRUB == nil {
			return 0, errors.New("фиксированная ставка без value_rub")
		}
		return *r.ValueRUB, nil
	case cpa.RatePercent:
		if r.ValuePercent == nil {
			return 0, errors.New("процентная ставка без value_percent")
		}
		return percentOf(sum, string(*r.ValuePercent))
	default:
		return 0, fmt.Errorf("вид ставки %q вне словаря", r.Kind)
	}
}

func percentOf(base cpa.RUB, percent string) (cpa.RUB, error) {
	p, ok := new(big.Rat).SetString(percent)
	if !ok {
		return 0, fmt.Errorf("процент %q не число", percent)
	}
	v := new(big.Rat).Mul(new(big.Rat).SetInt64(int64(base)), p)
	v.Quo(v, big.NewRat(100, 1))
	if !v.IsInt() {
		return 0, fmt.Errorf("%s %% от %s ₽ не целое число рублей — выберите сумму, кратную доле", percent, base)
	}
	return cpa.RUB(v.Num().Int64()), nil
}

func checkTriple(revenue, payout, margin cpa.RUB) error {
	if payout+margin != revenue {
		return fmt.Errorf("тройка ставок не сходится: выплата %s + маржа %s ≠ выручка %s", payout, margin, revenue)
	}
	return nil
}

// checkAmounts — выплата и выручка карточки равны тем, что следуют из ставок.
func checkAmounts(card *cpa.ConversionDetail, pay, rev cpa.Rate, sum cpa.RUB) error {
	wantPay, err := expectedRUB(pay, sum)
	if err != nil {
		return fmt.Errorf("ставка партнёра: %w", err)
	}
	wantRev, err := expectedRUB(rev, sum)
	if err != nil {
		return fmt.Errorf("ставка рекламодателя: %w", err)
	}
	if card.PayoutRUB != wantPay {
		return fmt.Errorf("выплата партнёру %s ₽, по ставке причитается %s ₽", card.PayoutRUB, wantPay)
	}
	if card.RevenueRUB != wantRev {
		return fmt.Errorf("выручка %s ₽, по ставке рекламодателя причитается %s ₽", card.RevenueRUB, wantRev)
	}
	return checkTriple(card.RevenueRUB, card.PayoutRUB, card.MarginRUB)
}

func legBaseAmount(card *cpa.ConversionDetail, base cpa.AgentLegBase) cpa.RUB {
	if base == cpa.AgentLegFromPayout {
		return card.PayoutRUB
	}
	return card.MarginRUB
}

// checkAgentLeg — нога есть, названа база сети, сумма равна проценту от базы.
func checkAgentLeg(card *cpa.ConversionDetail, agent cpa.PartnerExternalID, networkBase cpa.AgentLegBase, percent json.Number) error {
	leg := card.AgentLeg
	if leg == nil {
		return errors.New("в карточке нет ноги рекрутёра (agent_leg = null)")
	}
	if leg.ExternalID != agent {
		return fmt.Errorf("нога рекрутёра начислена %s, ждали %s", leg.ExternalID, agent)
	}
	if leg.Base != networkBase {
		return fmt.Errorf("база ноги %q, а сеть объявляет %q", leg.Base, networkBase)
	}
	if leg.Percent == nil || leg.Percent.String() != percent.String() {
		return fmt.Errorf("процент ноги %v, заводили %s", ptrString(leg.Percent), percent)
	}
	want, err := percentOf(legBaseAmount(card, leg.Base), percent.String())
	if err != nil {
		return err
	}
	if leg.AmountRUB != want {
		return fmt.Errorf("нога рекрутёра %s ₽, а %s %% от %s ₽ — %s ₽",
			leg.AmountRUB, percent, legBaseAmount(card, leg.Base), want)
	}
	return nil
}

// checkHold — срок удержания указан и соответствует холду цели (±1 мин на
// расхождение часов).
func checkHold(card *cpa.ConversionDetail, holdDays int, now time.Time) error {
	if card.HoldUntil == nil {
		return errors.New("срок удержания не указан (hold_until = null)")
	}
	if holdDays <= 0 {
		return nil
	}
	want := card.CreatedAt.Add(time.Duration(holdDays) * 24 * time.Hour)
	if d := card.HoldUntil.Sub(want); d > time.Minute || d < -time.Minute {
		return fmt.Errorf("холд до %s, а по условиям цели (%d дн.) — %s",
			card.HoldUntil.Format(time.RFC3339), holdDays, want.Format(time.RFC3339))
	}
	return nil
}

// goalRates — ставки цели в обеих проекциях.
func goalRates(ctx context.Context, c *cpa.Client, offerID, goal string) (pay, rev cpa.Rate, err error) {
	partnerView, _, err := c.Offers.GetForPartner(ctx, offerID)
	if err != nil {
		return pay, rev, fmt.Errorf("оффер %s: %w", offerID, err)
	}
	advertiserView, _, err := c.Offers.GetForAdvertiser(ctx, offerID, partnerView.AdvertiserID)
	if err != nil {
		return pay, rev, fmt.Errorf("оффер %s глазами рекламодателя: %w", offerID, err)
	}
	for _, g := range partnerView.Goals {
		if g.Code == goal && g.PayoutRate != nil {
			pay = *g.PayoutRate
		}
	}
	for _, g := range advertiserView.Goals {
		if g.Code == goal && g.RevenueRate != nil {
			rev = *g.RevenueRate
		}
	}
	if pay.Kind == "" || rev.Kind == "" {
		return pay, rev, fmt.Errorf("у оффера %s нет платной цели %q", offerID, goal)
	}
	return pay, rev, nil
}

func goalHoldDays(offer *cpa.OfferPartnerView, goal string) int {
	for _, g := range offer.Goals {
		if g.Code == goal && g.HoldDays != nil {
			return *g.HoldDays
		}
	}
	if offer.HoldDays != nil {
		return *offer.HoldDays
	}
	return 0
}

func ptrString[T ~string](p *T) string {
	if p == nil {
		return "null"
	}
	return string(*p)
}
