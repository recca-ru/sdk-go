package federation

import (
	"context"
	"net/http"

	"go.recca.ru/sdk/federationtypes"
)

// SelfService — сведения о самой сети и о предъявленном ключе.
type SelfService service

// Get отдаёт профиль вызывающей сети.
//
//	GET /federation/v1/self — права не требуется
//
// Права нет намеренно: дойдя до этого обработчика, ключ уже аутентифицирован,
// и скрывать от сети её собственный профиль было бы бессмысленно.
//
// ⚠⚠ Именно этим методом набор приёмки спрашивает СРЕДУ. Адрес, настроенный у
// клиента, свойством сервера не является: его берут из переменной окружения, а
// переменные путают. Ответ двери перепутать нельзя.
func (s *SelfService) Get(ctx context.Context) (*federationtypes.SelfInfo, *Response, error) {
	return do[federationtypes.SelfInfo](ctx, (*service)(s), request{
		Method:  http.MethodGet,
		Path:    "/self",
		Subject: "профиль сети",
	})
}
