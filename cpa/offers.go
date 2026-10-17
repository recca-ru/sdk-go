package cpa

import (
	"context"
	"net/http"
	"net/url"
)

const offersPath = "/cpa/offers"

// OffersService — каталог офферов и их материалы.
//
// ⚠⚠ У оффера ДВЕ проекции, и здесь они разведены ИМЕНАМИ МЕТОДОВ, а не
// параметром. Довод в контракте (раздел 9.3): партнёр, увидевший ставку
// рекламодателя, вычисляет маржу оператора, — то есть это не два вида одного
// ответа, а два разных ответа. Параметром проекцию можно забыть и получить
// умолчание двери; именем — нельзя, и тип возврата тогда честно разный.
type OffersService service

// viewQuery дописывает проекцию к фильтрам. Копия параметра одна: разойдись две,
// и один из методов однажды запросил бы чужую проекцию.
func viewQuery(params ListOffersParams, view string) url.Values {
	q := params.Query()
	q.Set("view", view)
	return q
}

// ListForPartner читает каталог в ПАРТНЁРСКОЙ проекции: GET /cpa/offers?view=partner.
//
// Здесь есть payout_rate, цели, гео и правила трафика — всё, что нужно блогеру,
// чтобы решить, брать ли оффер. Ставки рекламодателя и маржи здесь нет.
func (s *OffersService) ListForPartner(ctx context.Context, params ListOffersParams) ([]OfferPartnerView, *Response, error) {
	if err := params.Validate(); err != nil {
		return nil, nil, err
	}

	return doList[OfferPartnerView](ctx, (*service)(s), request{
		Method:  http.MethodGet,
		Path:    offersPath,
		Query:   viewQuery(params, "partner"),
		Subject: "каталог офферов",
	})
}

// ListForAdvertiser читает офферы одного рекламодателя в ЕГО проекции:
// GET /cpa/offers?view=advertiser&advertiser_id=….
//
// Здесь есть его собственная revenue_rate; payout_rate и маржа оператора — нет.
//
// ⚠ advertiserID — обязательный аргумент метода, а не поле фильтра: без него
// дверь отвечает 400 INVALID_INPUT (`advertiser_id is required when
// view=advertiser`). Довод тот же, что у разведения проекций по именам: поле можно
// забыть, аргумент — нет. Неизвестный идентификатор — 404 ADVERTISER_NOT_FOUND,
// а не пустая страница.
//
// params.AdvertiserID здесь не нужен; задан — обязан совпадать с advertiserID,
// иначе отказ до сети: два разных ответа на вопрос «чьи офферы» — это ошибка
// вызывающего, и молча выбрать один из них значило бы угадывать.
func (s *OffersService) ListForAdvertiser(
	ctx context.Context,
	advertiserID string,
	params ListOffersParams,
) ([]OfferAdvertiserView, *Response, error) {
	if advertiserID == "" {
		return nil, nil, validationError("advertiser_id",
			"обязательный аргумент: проекция рекламодателя без него дверью не отдаётся")
	}
	if params.AdvertiserID != "" && params.AdvertiserID != advertiserID {
		return nil, nil, validationError("advertiser_id",
			"задан дважды и по-разному: аргументом "+advertiserID+" и в фильтре "+params.AdvertiserID)
	}
	if err := params.Validate(); err != nil {
		return nil, nil, err
	}

	params.AdvertiserID = advertiserID
	return doList[OfferAdvertiserView](ctx, (*service)(s), request{
		Method:  http.MethodGet,
		Path:    offersPath,
		Query:   viewQuery(params, "advertiser"),
		Subject: "офферы рекламодателя",
	})
}

// GetForPartner читает оффер в партнёрской проекции: GET /cpa/offers/{id}?view=partner.
func (s *OffersService) GetForPartner(ctx context.Context, offerID string) (*OfferPartnerView, *Response, error) {
	if offerID == "" {
		return nil, nil, validationError("offer_id", "обязательное поле")
	}

	return do[OfferPartnerView](ctx, (*service)(s), request{
		Method:  http.MethodGet,
		Path:    offersPath + "/" + pathSegment(offerID),
		Query:   url.Values{"view": {"partner"}},
		Subject: "оффер",
	})
}

// GetForAdvertiser читает оффер в проекции рекламодателя:
// GET /cpa/offers/{id}?view=advertiser&advertiser_id=….
//
// ⚠⚠ С 02.10.2026 (Recca WV-0828 Д2R1-07) дверь требует advertiser_id и на
// КАРТОЧКЕ, как на каталоге, — и он обязан называть рекламодателя ЭТОГО
// оффера: чужой идентификатор открывал бы revenue_rate чужой карточки
// перебором. До этого метод жил без аргумента, и сигнатура сломана
// НАМЕРЕННО: обязательность, спрятанная в опциях, забывается; аргумент — нет.
// Неизвестный или чужой рекламодатель — 404 ADVERTISER_NOT_FOUND; взять
// идентификатор можно из партнёрской проекции того же оффера (поле
// AdvertiserID у GetForPartner/ListForPartner).
func (s *OffersService) GetForAdvertiser(
	ctx context.Context,
	offerID string,
	advertiserID string,
) (*OfferAdvertiserView, *Response, error) {
	switch {
	case offerID == "":
		return nil, nil, validationError("offer_id", "обязательное поле")
	case advertiserID == "":
		return nil, nil, validationError("advertiser_id",
			"обязательный аргумент: проекция рекламодателя без него дверью не отдаётся (Recca, октябрь 2026)")
	}

	query := url.Values{"view": {"advertiser"}, "advertiser_id": {advertiserID}}
	return do[OfferAdvertiserView](ctx, (*service)(s), request{
		Method:  http.MethodGet,
		Path:    offersPath + "/" + pathSegment(offerID),
		Query:   query,
		Subject: "оффер рекламодателя",
	})
}

// Creatives читает материалы оффера: GET /cpa/offers/{id}/creatives.
//
// ⚠ Посадочные и креативы приезжают ОДНИМ списком, посадочные первыми: без ленда
// трафик вести некуда. Ветвиться — по Kind: у CreativeText заполнен Text, а URL
// там null по построению, и обращение к нему без проверки уронило бы вызывающего.
func (s *OffersService) Creatives(ctx context.Context, offerID string) ([]Creative, *Response, error) {
	if offerID == "" {
		return nil, nil, validationError("offer_id", "обязательное поле")
	}

	return doList[Creative](ctx, (*service)(s), request{
		Method:  http.MethodGet,
		Path:    offersPath + "/" + pathSegment(offerID) + "/creatives",
		Subject: "материалы оффера",
	})
}
