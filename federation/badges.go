package federation

import (
	"context"
	"net/http"

	"go.recca.ru/sdk/federationtypes"
)

// BadgesService — бейджи ролей в сети.
type BadgesService service

// RequestPublic просит человека раскрыть бейдж.
//
//	POST /federation/v1/badges/:id/request-public — право `badges:write`
//
// ⚠⚠ Дверь ничего не меняет: она ОТПРАВЛЯЕТ ПРОСЬБУ. Requested = true значит
// «человеку ушло уведомление», а не «бейдж стал публичным». Видимость
// двусторонняя — своей половиной распоряжается сеть, своей человек, и
// действует та, что строже. Badge в ответе приезжает в ПРЕЖНЕМ состоянии;
// проверять надо EffectiveVisibility, и он не изменится, пока человек не
// согласится.
//
// ⚠⚠ Честный предел: эта дверь выводит код отказа из ТЕКСТА сообщения
// воркфлоу (`mapWorkflowError`), а не из собственного кода. Практически это
// значит, что на CONFLICT отсюда нельзя ветвиться как на контракте: правка
// формулировки в чужом воркфлоу молча сменит код. Дверей с таким поведением
// осталось четыре, все — бейджи, и список только тает.
//
// ⚠ Повтор не объявлен идемпотентным: различителя у операции нет, а вторая
// просьба — это второе уведомление человеку. Потерять повтор здесь дешевле,
// чем прислать человеку два одинаковых сообщения.
func (s *BadgesService) RequestPublic(
	ctx context.Context,
	badgeID string,
) (*federationtypes.RequestPublicResult, *Response, error) {
	if badgeID == "" {
		return nil, nil, validationError("badge_id", "обязательное поле")
	}
	return do[federationtypes.RequestPublicResult](ctx, (*service)(s), request{
		Method:  http.MethodPost,
		Path:    "/badges/" + pathSegment(badgeID) + "/request-public",
		Subject: "просьба раскрыть бейдж",
	})
}
