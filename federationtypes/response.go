package federationtypes

import (
	"net/url"
	"strconv"

	"go.recca.ru/sdk/reccatypes"
)

// Конверт ответа — общий у всех дверей Recca, поэтому живёт в reccatypes.
// Здесь псевдонимы: значение то же самое, структура, собранная под именем
// reccatypes.Response, ложится туда, где объявлен federationtypes.Response.
type (
	// Meta — блок meta конверта.
	//
	// ⚠⚠ У федеративных дверей он несёт ТОЛЬКО пагинацию: общий
	// `successResponse` не кладёт туда request_id никогда, а на отказах блока
	// нет вовсе. Идентификатор запроса читать из Response.RequestID.
	Meta = reccatypes.Meta
	// Response — статус, request_id, meta и заголовки ответа.
	Response = reccatypes.Response
	// RUB — сумма в рублях целым числом. Здесь она НЕ используется в типах
	// контракта (дверь принимает дробное), но нужна тем, кто ведёт свой учёт.
	RUB = reccatypes.RUB
)

// Page — параметры страницы федеративных списков.
//
// ⚠⚠ Своя структура, а не reccatypes.PageParams, и это не дубль: федеративные
// двери называют смещение `skip`, а CPA — `offset` (замер схем: `QuerySchema`
// в `offers/route.ts` и `users/:id/leads/route.ts` против `ListSchema` у CPA).
// Подставь сюда общий тип — и параметр уехал бы под чужим именем, а дверь
// молча вернула бы первую страницу вместо запрошенной. Молчаливая пагинация
// «всегда с начала» — ровно тот отказ, который замечают по жалобе.
type Page struct {
	// Take — сколько строк вернуть. 0 означает «умолчание двери» (20), а не
	// «ноль строк». Потолок двери — 100.
	Take int
	// Skip — сколько строк пропустить.
	Skip int
}

// AddTo дописывает страницу в строку запроса. Ноль не пишется: пустой ключ
// отличим от заданного, и дверь применяет своё умолчание.
func (p Page) AddTo(q url.Values) {
	if p.Take != 0 {
		q.Set("take", strconv.Itoa(p.Take))
	}
	if p.Skip != 0 {
		q.Set("skip", strconv.Itoa(p.Skip))
	}
}

// Validate — единственное, что клиент утверждает о странице сам: отрицательные
// величины бессмысленны при любом потолке двери. Сам потолок не проверяется —
// третья копия числа разошлась бы с дверью, а дверь отбивает 400 и называет
// поле.
func (p Page) Validate() error {
	if p.Take < 0 {
		return reccatypes.Invalid("take", "отрицательный размер страницы")
	}
	if p.Skip < 0 {
		return reccatypes.Invalid("skip", "отрицательное смещение")
	}
	return nil
}
