package federationtypes

// CreateReferralLinkInput — тело POST /federation/v1/users/:id/referral-links.
type CreateReferralLinkInput struct {
	// OfferID — оффер, на который чеканится ссылка.
	OfferID string `json:"offer_id"`
}

// Validate — то, что клиент утверждает сам, не обращаясь к двери.
func (in CreateReferralLinkInput) Validate() error {
	if in.OfferID == "" {
		return invalid("offer_id", "обязательное поле")
	}
	return nil
}

// ReferralLink — выданная ссылка.
//
// ⚠⚠ Дверь ИДЕМПОТЕНТНА по паре «человек ⇆ оффер», но без ключа в теле: второй
// вызов с тем же offer_id вернёт ТУ ЖЕ ссылку и статус 200 вместо 201. Заводить
// свой ключ идемпотентности здесь незачем и нечем — дверь его не примет.
//
// ⚠ Признака «переиспользовали» в ТЕЛЕ ответа нет: его несёт только код
// состояния. Поэтому Reused заполняет клиент по статусу — см. Reused.
type ReferralLink struct {
	// URL — длинный адрес посадочной страницы с меткой. Для встраивания на
	// свои страницы.
	URL string `json:"url" federation:"required"`
	// ShortURL — короткий адрес (recca.ru/d/<код>, около 30 знаков). Для чата,
	// СМС и QR.
	//
	// ⚠ По нему человек попадает на ту же посадочную, но метка подписывается
	// ЗАНОВО в момент перехода: срок жизни ссылки от этого не зависит.
	ShortURL string `json:"short_url"`
	// SignedRef — сама подписанная метка. Её же присылают обратно в
	// conversions.Register и leads.Create.
	SignedRef string `json:"signed_ref" federation:"required"`
	// DeeplinkID — идентификатор ссылки в Recca.
	DeeplinkID string `json:"deeplink_id" federation:"required"`

	// Reused — ссылка уже существовала и выдана повторно. Заполняется
	// КЛИЕНТОМ по коду ответа (200 против 201): в теле этого признака нет.
	Reused bool `json:"-"`
}
