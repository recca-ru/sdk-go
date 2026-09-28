package federation

import (
	"context"
	"net/http"
	"net/url"

	"go.recca.ru/sdk/federationtypes"
)

// OffersService — каталог офферов сети.
type OffersService service

// ListOffersParams — отбор каталога.
type ListOffersParams struct {
	// Page — размер и смещение страницы. Умолчание двери — 20, потолок 100.
	Page federationtypes.Page
	// ShopID — сузить до одной витрины. Витрина обязана принадлежать этой
	// сети; чужая даёт пустой список, а не отказ.
	ShopID string
	// Query — подстрока в заголовке, без учёта регистра.
	Query string
}

// List перечисляет офферы, которые сеть может продвигать.
//
//	GET /federation/v1/offers — право `offers:read`
//
// Дверь отдаёт ТОЛЬКО офферы с публичной видимостью: черновики не уходят
// наружу даже владельцу.
//
// ⚠ Здесь Meta.Total — настоящее общее число (дверь считает его отдельным
// запросом), в отличие от страницы заявок участника, где total равен размеру
// выданной страницы. Полагаться на одинаковое поведение двух списков нельзя.
func (s *OffersService) List(
	ctx context.Context,
	params ListOffersParams,
) ([]federationtypes.Offer, *Response, error) {
	if err := params.Page.Validate(); err != nil {
		return nil, nil, err
	}

	q := url.Values{}
	params.Page.AddTo(q)
	if params.ShopID != "" {
		q.Set("shop_id", params.ShopID)
	}
	if params.Query != "" {
		q.Set("q", params.Query)
	}

	return doWrappedList[federationtypes.Offer](ctx, (*service)(s), request{
		Method:  http.MethodGet,
		Path:    "/offers",
		Query:   q,
		Subject: "страница каталога офферов",
	}, "offers")
}
