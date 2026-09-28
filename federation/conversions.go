package federation

import (
	"context"
	"net/http"

	"go.recca.ru/sdk/federationtypes"
)

// ConversionsService — приём результата и его отмена.
type ConversionsService service

// Register засчитывает конверсию и платит каскаду.
//
//	POST /federation/v1/conversions — право `conversions:write`
//
// ⚠⚠ Ключ идемпотентности — ExternalID В ТЕЛЕ. Повтор с тем же значением
// возвращает 200 и Idempotent = true, НЕ платя второй раз. Заголовка
// `Idempotency-Key` дверь не читает; клиент его не шлёт.
//
// ⚠⚠ Именно поэтому повтор здесь разрешён транспорту. Без различителя в теле
// это была бы непростительная ошибка: ответ мог потеряться уже ПОСЛЕ того, как
// каскад оплачен, и слепой повтор оплатил бы его вторично.
//
// ⚠ Пространство ключей на «recca:» зарезервировано за внутренними конверсиями
// платформы — дверь отобьёт такой ExternalID кодом RESERVED_EXTERNAL_ID.
// Отказ громкий намеренно: заняв чужой ключ, арендатор получил бы молчаливый
// успех, не заплативший никому.
func (s *ConversionsService) Register(
	ctx context.Context,
	in federationtypes.RegisterConversionInput,
) (*federationtypes.RegisterConversionResult, *Response, error) {
	if err := in.Validate(); err != nil {
		return nil, nil, err
	}
	return do[federationtypes.RegisterConversionResult](ctx, (*service)(s), request{
		Method:          http.MethodPost,
		Path:            "/conversions",
		Body:            in,
		Subject:         "конверсия",
		IdempotentWrite: true,
	})
}

// Reverse отменяет конверсию целиком или частично.
//
//	POST /federation/v1/conversions/:external_id/reverse
//	                                     — право `conversions:reverse`
//
// Право отдельное от `conversions:write` намеренно: ключ, которым сеть
// засчитывает результат, не обязан уметь забирать деньги обратно.
//
// ⚠⚠ Дверь отвечает 200 и при ЧАСТИЧНОЙ отмене — полнота в теле, а не в
// статусе. Ноги со статусом «выплачено» отменить нельзя, и они приезжают в
// UnreversedLegs с причиной у каждой. Прочитать 200 как «сняли всё» значит
// списать у себя больше, чем сняли у нас; для этого вопроса есть
// ReverseConversionResult.FullyReversed().
//
// ⚠⚠ ВТОРОЙ и последующий ЧАСТИЧНЫЙ откат обязан нести свой ReversalID. Без
// него два частичных отката делят один ключ идемпотентности, и второй вернёт
// результат первого — то есть отчитается о деньгах, которых не двигал.
func (s *ConversionsService) Reverse(
	ctx context.Context,
	externalID string,
	in federationtypes.ReverseConversionInput,
) (*federationtypes.ReverseConversionResult, *Response, error) {
	switch {
	case externalID == "":
		return nil, nil, validationError("external_id", "обязательное поле")
	case len(externalID) > 200:
		return nil, nil, validationError("external_id", "длиннее 200 символов")
	}
	if err := in.Validate(); err != nil {
		return nil, nil, err
	}

	return do[federationtypes.ReverseConversionResult](ctx, (*service)(s), request{
		Method:  http.MethodPost,
		Path:    "/conversions/" + pathSegment(externalID) + "/reverse",
		Body:    in,
		Subject: "отмена конверсии",
		// Идемпотентность держит ключ на ногу (`rev:{сеть}:{external_id}:L{n}`,
		// с именем отката у второго частичного) — повтор безопасен.
		IdempotentWrite: true,
	})
}
