package cpatypes

import (
	"net/url"

	"go.recca.ru/sdk/reccatypes"
)

// PageParams — общая часть параметров страницы. Определение — в reccatypes:
// страница у всех списков Recca устроена одинаково.
//
// ⚠⚠ Потолки limit и offset клиент НЕ проверяет, и это решение, а не пропуск —
// довод записан у определения.
type PageParams = reccatypes.PageParams

// setIfNotEmpty — короткая форма для необязательного параметра строки запроса.
// Обёртка, а не вторая реализация: правило «пустое не пишем» одно на модуль.
func setIfNotEmpty(q url.Values, key, value string) {
	reccatypes.SetIfNotEmpty(q, key, value)
}
