package webhook

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrBadSignature — подпись не сошлась либо запрос вне допуска свежести.
	ErrBadSignature = errors.New("webhook: подпись не сошлась")
	// ErrDuplicate — событие с этим event_id уже обрабатывалось.
	//
	// ⚠ Это НЕ ошибка: повтор доставки — штатное поведение Recca, и правильный
	// ответ на него — 200 без работы. Отдельное значение нужно именно для
	// того, чтобы «уже сделано» не выглядело как «не получилось» и не
	// заставляло нас доставлять снова.
	ErrDuplicate = errors.New("webhook: событие уже обработано")
)

// SeenStore — память о том, какие события уже обработаны.
//
// MarkSeen отмечает событие и отвечает, было ли оно НОВЫМ. Вызов обязан быть
// атомарным: две доставки одного события приходят параллельно (ретрай обгоняет
// первую попытку), и проверка отдельно от записи пропустила бы обе.
//
// ⚠⚠ Правильная промышленная реализация — уникальный индекс в вашей базе по
// `event_id` и вставка, чей конфликт означает «уже было». Так это делает сама
// Recca на своей стороне. Реализация в памяти (NewMemorySeen) промышленной НЕ
// является — см. её собственное предупреждение.
type SeenStore interface {
	MarkSeen(ctx context.Context, eventID string) (fresh bool, err error)
}

// Receiver — приём входящего вебхука: подпись, конверт, защита от повтора.
type Receiver struct {
	// Secret — секрет вебхука вашей сети.
	Secret string
	// Tolerance — допуск свежести; ноль означает 5 минут (порог платформы).
	Tolerance time.Duration
	// Seen — память обработанных событий. nil означает «защиты от повтора
	// нет».
	//
	// ⚠⚠ nil разрешён намеренно и ровно для одного случая: ваш обработчик
	// идемпотентен САМ (пишет по ключу, а не прибавляет). Если он начисляет,
	// шлёт письмо или двигает счётчик — nil здесь означает, что повтор
	// доставки сделает это дважды, и ни подпись, ни свежесть этого не
	// остановят: у повтора ЗАКОННАЯ свежая подпись.
	Seen SeenStore
	// Now — «сейчас» для проверки свежести; nil означает time.Now.
	Now func() time.Time
}

// DefaultTolerance — допуск свежести платформы.
const DefaultTolerance = 5 * time.Minute

// Accept проверяет вебхук целиком и отвечает, что с ним делать.
//
// Порядок шагов — часть защиты, а не удобство:
//
//  1. ПОДПИСЬ. До неё тело — данные неизвестного происхождения.
//  2. КОНВЕРТ. Разбор подписанного тела.
//  3. ПОВТОР. Отметка event_id.
//
// ⚠⚠ Переставить 3 перед 1 — дыра, и неочевидная: кто угодно, зная ваш адрес,
// послал бы вам придуманные `event_id` и ЗАБИЛ БЫ ИМИ память обработанных.
// После этого настоящая доставка с тем же идентификатором пришла бы как
// «повтор», вы ответили бы 200, и событие пропало бы молча — без ошибки, без
// ретрая, без следа.
//
// Возвращает ErrDuplicate на повторе. Это не отказ: ответьте Recca 200 и не
// делайте работу второй раз.
func (r Receiver) Accept(ctx context.Context, header string, body []byte) (Envelope, error) {
	tolerance := r.Tolerance
	if tolerance == 0 {
		tolerance = DefaultTolerance
	}
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}

	// 1. Подпись — по СЫРОМУ телу, байт в байт.
	if !Verify(r.Secret, header, body, now(), tolerance) {
		return Envelope{}, ErrBadSignature
	}

	// 2. Конверт.
	env, err := Parse(body)
	if err != nil {
		return Envelope{}, err
	}

	// 3. Повтор.
	if r.Seen == nil {
		return env, nil
	}
	fresh, err := r.Seen.MarkSeen(ctx, env.EventID)
	if err != nil {
		// ⚠⚠ Отказ памяти — это НЕ «событие новое». Ответить успехом здесь
		// значило бы обработать повтор, пока база недоступна, то есть ровно
		// тогда, когда всё и так плохо. Пусть Recca доставит ещё раз.
		return env, fmt.Errorf("webhook: не удалось проверить повтор события %s: %w", env.EventID, err)
	}
	if !fresh {
		return env, ErrDuplicate
	}
	return env, nil
}

// MemorySeen — память обработанных событий В ПРОЦЕССЕ.
//
// ⚠⚠ Для боя она НЕ годится, и это не осторожность, а арифметика:
//
//   - перезапуск процесса стирает её целиком, а Recca повторяет доставку
//     часами;
//   - за вторым экземпляром приложения она не видна вовсе, то есть под
//     балансировщиком две копии обработают одно событие каждая.
//
// Оба случая дают ДВОЙНУЮ обработку — то самое, ради предотвращения чего её и
// подключают. Годится она для тестов и для одного процесса, которому дороже
// защита от быстрого ретрая, чем полная.
//
// Промышленный вариант — уникальный индекс по `event_id` в вашей базе.
type MemorySeen struct {
	ttl time.Duration
	now func() time.Time

	mu   sync.Mutex
	seen map[string]time.Time
}

// NewMemorySeen заводит память с временем жизни записи. Ноль означает сутки:
// дольше Recca не повторяет.
func NewMemorySeen(ttl time.Duration) *MemorySeen {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &MemorySeen{ttl: ttl, now: time.Now, seen: map[string]time.Time{}}
}

// MarkSeen отмечает событие и отвечает, было ли оно новым.
func (m *MemorySeen) MarkSeen(_ context.Context, eventID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	// Уборка просроченного здесь же: отдельная горутина пережила бы
	// потребителя, который про неё не просил, а карта иначе растёт вечно.
	for id, at := range m.seen {
		if now.Sub(at) > m.ttl {
			delete(m.seen, id)
		}
	}

	if at, ok := m.seen[eventID]; ok && now.Sub(at) <= m.ttl {
		return false, nil
	}
	m.seen[eventID] = now
	return true, nil
}
