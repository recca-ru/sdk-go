package federation

import (
	"context"
	"net/http"

	"go.recca.ru/sdk/federationtypes"
)

// ReferralLinksService — чеканка реферальных ссылок от имени участника.
type ReferralLinksService service

// Create чеканит (или переиспользует) ссылку участника на оффер.
//
//	POST /federation/v1/users/:id/referral-links — право `users:impersonate`
//
// Это главный путь модели B: всплывающего окна согласия у человека нет,
// потому что сеть уже знает его по своей CRM и держит ключ с правом
// действовать от его имени.
//
// ⚠⚠ Дверь ИДЕМПОТЕНТНА по паре «человек ⇆ оффер»: второй вызов вернёт ту же
// ссылку и 200 вместо 201. Признака в теле нет — он только в коде состояния, —
// поэтому его проставляет клиент в поле Reused. Заводить свой ключ
// идемпотентности здесь нечем: дверь его не читает.
func (s *ReferralLinksService) Create(
	ctx context.Context,
	userID string,
	in federationtypes.CreateReferralLinkInput,
) (*federationtypes.ReferralLink, *Response, error) {
	if userID == "" {
		return nil, nil, validationError("user_id", "обязательное поле")
	}
	if err := in.Validate(); err != nil {
		return nil, nil, err
	}

	link, resp, err := do[federationtypes.ReferralLink](ctx, (*service)(s), request{
		Method:  http.MethodPost,
		Path:    "/users/" + pathSegment(userID) + "/referral-links",
		Body:    in,
		Subject: "реферальная ссылка",
		// Повтор отдаёт ТУ ЖЕ ссылку: второй строки не появится.
		IdempotentWrite: true,
	})
	if err != nil {
		return nil, resp, err
	}

	// ⚠ Признак живёт в статусе, и прочитать его можно только здесь: наружу
	// метод отдаёт уже разобранную структуру, а из неё код состояния не виден.
	link.Reused = resp.StatusCode == http.StatusOK
	return link, resp, nil
}
