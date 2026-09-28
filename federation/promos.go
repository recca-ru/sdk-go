package federation

import (
	"context"
	"net/http"

	"go.recca.ru/sdk/federationtypes"
)

// PromosService — деньги участника по программам сети.
type PromosService service

// Balance отдаёт баланс участника в разрезе валют.
//
//	GET /federation/v1/users/:id/bonus-balance — право `bonuses:read`
//
// ⚠⚠ Дверь отвечает на ДВА РАЗНЫХ ВОПРОСА, и спутать их дорого:
//
//	LifetimeNet   — «начислено за всё время минус отменено». Вывод денег
//	                его НЕ уменьшает. В JSON это поле зовётся `available`,
//	                и имя читается ровно наоборот смыслу.
//	WalletCleared — «сколько человек может потратить СЕЙЧАС». Вывод
//	                уменьшает именно его.
//
// До первого вывода числа совпадают, после первого расходятся навсегда.
// Показывать человеку LifetimeNet как остаток — значит показать ему деньги,
// которые он уже получил.
//
// ⚠⚠ WalletCleared может быть nil, и это значит «дверь не знает», а НЕ «ноль».
// Так бывает, когда счёт человека несёт операции ещё какой-то сети: баланс
// счёта — одно число, разделить его между сетями нечем. Для этого вопроса есть
// Balance.Spendable(), возвращающий вторым значением «знает ли».
func (s *PromosService) Balance(
	ctx context.Context,
	userID string,
) (*federationtypes.Balance, *Response, error) {
	if userID == "" {
		return nil, nil, validationError("user_id", "обязательное поле")
	}
	return do[federationtypes.Balance](ctx, (*service)(s), request{
		Method:  http.MethodGet,
		Path:    "/users/" + pathSegment(userID) + "/bonus-balance",
		Subject: "баланс участника",
	})
}
