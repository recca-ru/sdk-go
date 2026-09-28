package cpatypes

import "go.recca.ru/sdk/reccatypes"

// Конверт ответа — общий у всех дверей Recca, поэтому живёт в reccatypes.
// Здесь псевдонимы: значение то же самое, структура, собранная под именем
// reccatypes.Response, ложится туда, где объявлен cpatypes.Response.
type (
	// Meta — блок meta конверта (раздел 4).
	Meta = reccatypes.Meta
	// Response — статус, request_id, meta и заголовки ответа.
	Response = reccatypes.Response
)
