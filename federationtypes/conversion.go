package federationtypes

// AttributionSource — каким путём дверь узнала, кому платить.
type AttributionSource string

const (
	// AttributionTree — по подписанной реферальной метке и дереву федерации.
	AttributionTree AttributionSource = "federation_tree"
	// AttributionTenantReferral — по промокоду либо метке привлечения
	// собственной реферальной программы сети.
	AttributionTenantReferral AttributionSource = "tenant_referral"
)

// ConversionLeg — одна нога каскада: кому и сколько заплатили на этом колене.
type ConversionLeg struct {
	// Level — колено, начиная с 1 (тот, кто привёл непосредственно).
	Level int `json:"level" federation:"required"`
	// RecipientUserID — получатель.
	RecipientUserID string `json:"recipient_user_id" federation:"required"`
	// AmountRub — сумма ноги.
	AmountRub float64 `json:"amount_rub" federation:"required"`
	// BonusTransactionID — строка начисления; пусто, если начисления не было.
	BonusTransactionID string `json:"bonus_transaction_id"`
}

// RegisterConversionInput — тело POST /federation/v1/conversions.
//
// ⚠⚠ У двери ДВА взаимоисключающих пути привлечения, и выбирает их она по
// присланным полям:
//
//   - ReccaRef — дерево федерации;
//   - PromoCode либо AttributionToken (и обязательно UserID) — собственная
//     реферальная программа сети.
//
// Прислать и то и другое — 400: дверь отказывается гадать, кому платить.
// Проверку клиент делает сам, до сети, — чтобы отказ пришёл там, где его
// видно, а не в журнале.
type RegisterConversionInput struct {
	// EventType — что произошло: "purchase", "signup" и прочее.
	EventType string `json:"event_type"`
	// ExternalID — идентификатор события НА СТОРОНЕ АРЕНДАТОРА.
	//
	// ⚠⚠ Это и есть ключ идемпотентности конверсии. Повтор с тем же значением
	// возвращает 200 и Idempotent = true, не платя второй раз. Заголовка
	// `Idempotency-Key` дверь не читает — генерировать ExternalID заново на
	// каждую попытку значит отключить защиту от двойной оплаты.
	//
	// ⚠ Пространство ключей, начинающееся на «recca:», зарезервировано за
	// внутренними конверсиями платформы: дверь отобьёт такой запрос кодом
	// RESERVED_EXTERNAL_ID.
	ExternalID string `json:"external_id"`
	// AmountRub — сумма события, от которой считается вознаграждение.
	AmountRub float64 `json:"amount_rub"`

	// ReccaRef — подписанная реферальная метка (путь дерева федерации).
	ReccaRef string `json:"recca_ref,omitempty"`

	// PromoCode — промокод собственной программы сети.
	PromoCode string `json:"promo_code,omitempty"`
	// AttributionToken — метка привлечения собственной программы сети.
	AttributionToken string `json:"attribution_token,omitempty"`
	// UserID — кто совершил событие. Обязателен на пути промокода и метки:
	// без него неизвестно, чьё погашение засчитывать.
	UserID string `json:"user_id,omitempty"`

	// Metadata — произвольный снимок события.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Validate — то, что клиент утверждает сам, не обращаясь к двери.
func (in RegisterConversionInput) Validate() error {
	if in.EventType == "" {
		return invalid("event_type", "обязательное поле")
	}
	if in.ExternalID == "" {
		return invalid("external_id", "обязательное поле")
	}

	promoPath := in.PromoCode != "" || in.AttributionToken != ""
	switch {
	case in.ReccaRef == "" && !promoPath:
		return invalid("recca_ref",
			"нужен recca_ref либо promo_code/attribution_token — иначе неизвестно, кто привёл")
	case in.ReccaRef != "" && promoPath:
		return invalid("recca_ref",
			"recca_ref нельзя сочетать с promo_code/attribution_token: у двери это разные пути привлечения")
	case promoPath && in.UserID == "":
		return invalid("user_id", "обязателен на пути промокода: без него неизвестно, чьё погашение засчитывать")
	}
	return nil
}

// RegisterConversionResult — ответ двери приёма конверсии.
//
// ⚠ Часть полей заполняется только на одном из двух путей привлечения:
// RedemptionID и ProgramID — на пути промокода, BonusAmountRub и
// BonusTransactionID — на пути дерева (там это нога первого колена, оставленная
// ради совместимости с WV-0149). AttributionSource называет, какой путь
// сработал, и ветвиться следует по нему, а не по пустоте полей.
type RegisterConversionResult struct {
	// ConversionID — идентификатор конверсии в Recca.
	ConversionID string `json:"conversion_id" federation:"required"`
	// Idempotent — конверсия с таким ExternalID уже была: это ПОВТОР, и
	// второй раз никто не заплатил.
	Idempotent bool `json:"idempotent" federation:"required"`
	// AttributionSource — каким путём дверь узнала, кому платить.
	AttributionSource AttributionSource `json:"attribution_source"`

	// CascadeAmountRub — сколько заплатили по всем коленам.
	CascadeAmountRub float64 `json:"cascade_amount_rub"`
	// CascadeLegsCount — сколько колен оплачено.
	CascadeLegsCount int `json:"cascade_legs_count"`
	// Legs — сами ноги.
	Legs []ConversionLeg `json:"legs"`
	// PartnerUserID — кто привёл.
	PartnerUserID string `json:"partner_user_id"`

	// BonusAmountRub — нога первого колена (путь дерева).
	BonusAmountRub float64 `json:"bonus_amount_rub"`
	// BonusTransactionID — её строка начисления (путь дерева).
	BonusTransactionID string `json:"bonus_transaction_id"`

	// RedemptionID — погашение промокода (путь промокода).
	RedemptionID string `json:"redemption_id"`
	// ProgramID — программа, по которой платили (путь промокода).
	ProgramID string `json:"program_id"`
}

// ReverseConversionInput — тело POST /federation/v1/conversions/:external_id/reverse.
type ReverseConversionInput struct {
	// AmountToReverse — сколько отменить. Ноль означает ПОЛНУЮ отмену.
	//
	// ⚠ Указатель: у частичной отмены на ноль смысла нет, а «не указали» —
	// обычное дело, и это разные распоряжения.
	AmountToReverse *float64 `json:"amount_to_reverse,omitempty"`
	// ReversalID — имя частичной отмены, [A-Za-z0-9_.:-] до 64 знаков.
	//
	// ⚠⚠ Второй и последующий ЧАСТИЧНЫЙ откат обязан нести своё имя. Без него
	// оба частичных отката делят один ключ идемпотентности, и второй вернёт
	// результат первого — то есть отчитается о деньгах, которых не двигал.
	ReversalID string `json:"reversal_id,omitempty"`
	// Reason — обоснование, 1..500 знаков. Обязательно: отмена денег без
	// причины не разбирается потом никем.
	Reason string `json:"reason"`
}

// Validate — то, что клиент утверждает сам, не обращаясь к двери.
func (in ReverseConversionInput) Validate() error {
	switch {
	case in.Reason == "":
		return invalid("reason", "обязательное поле")
	case len(in.Reason) > 500:
		return invalid("reason", "длиннее 500 символов")
	}
	if in.AmountToReverse != nil && *in.AmountToReverse < 0 {
		return invalid("amount_to_reverse", "отрицательная сумма")
	}
	return nil
}

// ReversedLeg — что стало с одной ногой при отмене.
type ReversedLeg struct {
	// Level — колено.
	Level int `json:"level" federation:"required"`
	// RecipientUserID — чья нога.
	RecipientUserID string `json:"recipient_user_id" federation:"required"`
	// ReversedAmountRub — сколько сняли.
	ReversedAmountRub float64 `json:"reversed_amount_rub" federation:"required"`
	// ReversalTransactionID — строка отмены; пусто, если её не было.
	ReversalTransactionID string `json:"reversal_transaction_id"`
	// UnreversedReason — почему ногу НЕ отменили. Заполняется только у ног из
	// UnreversedLegs: "ZERO_AMOUNT", "NO_BONUS_TX", уже выплачено и прочее.
	UnreversedReason string `json:"unreversed_reason"`
}

// ReverseConversionResult — ответ двери отмены.
//
// ⚠⚠ Дверь отвечает 200 и при ЧАСТИЧНОЙ отмене: полнота — в теле, а не в
// статусе. Прочитать 200 как «отменили всё» — значит списать со своей стороны
// больше, чем сняли с нашей. Смотреть надо Status и UnreversedLegs.
type ReverseConversionResult struct {
	// ConversionID — какую конверсию отменяли.
	ConversionID string `json:"conversion_id" federation:"required"`
	// Status — "reversed" (отменено всё) либо "partial_reversed".
	Status string `json:"status" federation:"required"`
	// ReversedAmountRub — сколько сняли всего.
	ReversedAmountRub float64 `json:"reversed_amount_rub" federation:"required"`
	// Legs — ноги, которые отменили.
	Legs []ReversedLeg `json:"legs"`
	// UnreversedLegs — ноги, которые отменить НЕ удалось, с причиной у
	// каждой. Непустой список при Status = "partial_reversed".
	UnreversedLegs []ReversedLeg `json:"unreversed_legs"`
	// Idempotent — отмена по этому ключу уже проходила.
	Idempotent bool `json:"idempotent"`
}

// FullyReversed отвечает, сняли ли всё.
//
// ⚠ Отдельный вопрос, потому что 200 на него не отвечает, а самый частый
// способ ошибиться — решить, что отвечает.
func (r ReverseConversionResult) FullyReversed() bool {
	return r.Status == "reversed" && len(r.UnreversedLegs) == 0
}
