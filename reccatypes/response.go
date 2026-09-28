package reccatypes

import (
	"encoding/json"
	"net/http"
)

// Meta — блок meta конверта ответа.
//
// Все поля страницы — указатели, потому что «не прислали» и «прислали ноль» у
// них разные: список из нуля строк и список, которого не считали, — разные
// ответы.
type Meta struct {
	// RequestID приходит и на успехе, и на отказе. Называйте его, сообщая о
	// проблеме: по нему находится вся цепочка на стороне Recca.
	//
	// ⚠⚠ Поле обещает КОНТРАКТ CPA-двери, а не платформа целиком. Остальная
	// федерация собирает meta общим `successResponse`, который кладёт туда
	// только пагинацию, — то есть на её ответах RequestID здесь ПУСТ, а на
	// отказах блока meta нет вовсе (замер кода дверей 28.09.2026:
	// `api/federation/v1/cpa/_lib/respond.ts` против `utils/api-response.ts`).
	//
	// Идентификатор при этом есть ВСЕГДА — заголовком `X-Request-Id`, который
	// ставит глобальный `requestIdMiddleware`. Поэтому читать его правильно из
	// Response.RequestID: тот берёт заголовок и падает на meta только если
	// заголовка не оказалось.
	RequestID string `json:"request_id"`
	Total     *int   `json:"total,omitempty"`
	Count     *int   `json:"count,omitempty"`
	Offset    *int   `json:"offset,omitempty"`
	Limit     *int   `json:"limit,omitempty"`

	// HasMore — есть ли ещё страница.
	//
	// ⚠⚠ Две двери одной платформы пишут это поле РАЗНЫМИ именами:
	// CPA-контракт — `has_more`, общий `successResponse` федерации — `hasMore`
	// (`utils/api-response.ts`, поле добавляется там же, где считается).
	// Расхождение живое и старше этого SDK; чинить его пришлось бы ломающей
	// правкой у витрины и JS-клиента, то есть не здесь.
	//
	// Клиент принимает обе формы (см. UnmarshalJSON) — иначе на половине
	// ответов поле молча оставалось бы nil, и «страниц больше нет» было бы
	// неотличимо от «дверь не сказала». Выдавать же одну из сторон за
	// неправильную SDK не вправе: обе работают в проде.
	HasMore *bool `json:"has_more,omitempty"`

	// Truncated со значением true означает, что выдача НЕПОЛНА: дверь упёрлась
	// в потолок просмотра, и Total — сколько успели посчитать, а не сколько
	// есть.
	//
	// ⚠⚠ У CPA-двери поле приходит всегда, включая false: «мы посмотрели всё» —
	// это утверждение, а не отсутствие оговорки. Поэтому указатель: иначе
	// неполная выдача была бы неотличима от полной. Остальная федерация
	// потолка просмотра не объявляет и поля не шлёт — там nil означает «дверь
	// об этом не говорит».
	Truncated *bool `json:"truncated,omitempty"`
}

// UnmarshalJSON разбирает meta, принимая обе живые формы признака следующей
// страницы. Приоритет у `has_more`: это форма объявленного контракта, а
// `hasMore` — форма общего ответчика, который контракта на себя не брал.
func (m *Meta) UnmarshalJSON(raw []byte) error {
	// ⚠ Тип-двойник без методов: разбирать Meta напрямую значило бы звать этот
	// же UnmarshalJSON и уйти в бесконечную рекурсию.
	type plain Meta
	var p plain
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	*m = Meta(p)

	if m.HasMore == nil {
		var camel struct {
			HasMore *bool `json:"hasMore"`
		}
		if err := json.Unmarshal(raw, &camel); err == nil && camel.HasMore != nil {
			m.HasMore = camel.HasMore
		}
	}
	return nil
}

// Response — то, что клиент знает об ответе помимо самих данных.
type Response struct {
	// StatusCode — HTTP-статус ответа.
	StatusCode int
	// RequestID — итоговый идентификатор запроса: заголовок X-Request-Id, если
	// он есть, иначе meta.request_id.
	//
	// ⚠ Приоритет у заголовка не случаен, и причин теперь две. Первая: два
	// отказа формируются до того, как запрос дойдёт до метода, и тела с meta у
	// них нет вовсе — «ключ не предъявлен» (401) и «исчерпан бюджет» (429).
	// Вторая: вся федерация, кроме CPA, не кладёт request_id в meta НИКОГДА.
	RequestID string
	// Meta — блок meta так, как его прислала дверь. Пуст, если тела не было.
	Meta Meta
	// Header — заголовки ответа: Retry-After, X-RateLimit-Limit,
	// X-RateLimit-Remaining и прочее, что нужно для собственного учёта.
	Header http.Header
}
