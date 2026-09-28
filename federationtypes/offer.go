package federationtypes

import (
	"encoding/json"
	"time"
)

// PriceMode — что означает цена оффера.
//
// ⚠⚠ Поле существует потому, что `price = 0` двусмысленно: это и «бесплатно»,
// и «цены нет». Читать цену, не прочитав режим, значит однажды напечатать
// «0 ₽» там, где написано «цена договорная».
type PriceMode string

const (
	// PriceFixed — цена названа числом.
	PriceFixed PriceMode = "fixed"
	// PriceFree — бесплатно.
	PriceFree PriceMode = "free"
	// PriceNegotiable — цена договорная; числу верить нельзя.
	PriceNegotiable PriceMode = "negotiable"
)

// Offer — оффер каталога сети, как его отдаёт GET /federation/v1/offers.
//
// Каталог содержит ТОЛЬКО офферы с `visibility = "public"` витрин этой сети:
// черновики не уходят наружу даже самому владельцу.
type Offer struct {
	// ID — идентификатор оффера. Им зовётся создание реферальной ссылки.
	ID string `json:"id" federation:"required"`
	// ShopID — витрина, которой принадлежит оффер.
	ShopID string `json:"shop_id" federation:"required"`
	// Slug — человекочитаемая часть адреса; пусто, если её нет.
	Slug string `json:"slug"`
	// Title — заголовок.
	Title string `json:"title" federation:"required"`
	// Description — описание.
	Description string `json:"description"`

	// Price — цена. ⚠ Указатель: «цены не назвали» и «цена ноль» — разные
	// утверждения, и дверь их различает.
	Price *float64 `json:"price"`
	// PriceMax — верхняя граница вилки; nil, если цена одна.
	PriceMax *float64 `json:"price_max"`
	// PriceMode — как читать Price. Пусто означает «дверь не сказала»; в этом
	// случае цена читается как fixed, но утверждать это за дверь нельзя.
	PriceMode PriceMode `json:"price_mode,omitempty"`
	// CurrencyCode — валюта цены.
	CurrencyCode string `json:"currency_code"`

	// Visibility — видимость. В каталоге федерации всегда "public": иного
	// дверь не отдаёт.
	Visibility string `json:"visibility"`
	// SiteURL — собственный адрес оффера на стороне витрины.
	SiteURL string `json:"site_url"`
	// Metadata — произвольные данные оффера.
	Metadata map[string]json.RawMessage `json:"metadata"`
	// CreatedAt — когда оффер завели.
	CreatedAt *time.Time `json:"created_at"`
}

// PriceKnown отвечает, можно ли показывать Price числом.
//
// ⚠ Отдельный вопрос, а не `Price != nil`: у режима negotiable число приезжает
// и врёт. Здесь оба условия сведены в одном месте, чтобы каждый потребитель не
// вспоминал о втором.
func (o Offer) PriceKnown() bool {
	return o.Price != nil && o.PriceMode != PriceNegotiable && o.PriceMode != PriceFree
}
