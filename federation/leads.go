package federation

import (
	"context"
	"net/http"

	"go.recca.ru/sdk/federationtypes"
)

// LeadsService — приём заявок.
type LeadsService service

// Create засчитывает заявку тому, кто привёл.
//
//	POST /federation/v1/leads — право `leads:write`
//
// ⚠⚠ Ключ идемпотентности — ExternalID В ТЕЛЕ, а не заголовок. Заголовка
// `Idempotency-Key` эта дверь не читает вовсе (как и все остальные двери
// федерации), поэтому клиент его и не шлёт: слать значило бы обещать
// поведение, которого нет. Повтор с тем же ExternalID вернёт 200 и
// Idempotent = true — второй заявки не появится, и второй раз никому не
// заплатят.
//
// Отсюда правило для вызывающего: ExternalID обязан быть идентификатором
// СОБЫТИЯ (номер отправки формы), а не попытки. Сгенерируй его заново на
// повторе — и защита выключена, хотя выглядит включённой.
func (s *LeadsService) Create(
	ctx context.Context,
	in federationtypes.CreateLeadInput,
) (*federationtypes.CreateLeadResult, *Response, error) {
	if err := in.Validate(); err != nil {
		return nil, nil, err
	}
	return do[federationtypes.CreateLeadResult](ctx, (*service)(s), request{
		Method:          http.MethodPost,
		Path:            "/leads",
		Body:            in,
		Subject:         "заявка",
		IdempotentWrite: true,
	})
}
