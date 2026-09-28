package webhook_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"go.recca.ru/sdk/webhook"
)

func ctx() context.Context { return context.Background() }

// caseByName достаёт вектор по имени; отсутствие — отказ, а не пропуск.
//
// ⚠ Пропущенный случай сделал бы тест зелёным по пустоте: именно так проверка
// перестаёт проверять после переименования вектора.
func caseByName(t *testing.T, v hookVectors, name string) hookCase {
	t.Helper()
	for _, c := range v.Cases {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("в векторах нет случая %q — проверять нечего", name)
	return hookCase{}
}

// ── конверт ──────────────────────────────────────────────────────────────────

func TestParseFederationEnvelope(t *testing.T) {
	v := load(t)
	c := caseByName(t, v, "federation_conversion_cascaded")

	env, err := webhook.Parse([]byte(c.RawBody))
	if err != nil {
		t.Fatalf("конверт федерации не разобрался: %v", err)
	}
	if env.EventType != "conversion.cascaded" {
		t.Errorf("event_type = %q", env.EventType)
	}
	if env.EventID == "" || env.OrganizationID == "" {
		t.Errorf("конверт неполон: %+v", env)
	}

	// ⚠⚠ «Когда случилось» и «когда доставлено» — РАЗНЫЕ факты, и при ретраях
	// расходятся на часы. Отчёт, построенный по доставке, меряет нашу очередь,
	// а не деньги.
	if env.OccurredAt == nil || env.DeliveredAt == nil {
		t.Fatalf("времена не разобраны: occurred=%v delivered=%v", env.OccurredAt, env.DeliveredAt)
	}

	// Полезная нагрузка остаётся сырой и разбирается ПОСЛЕ ветвления по типу.
	var data struct {
		ConversionID string `json:"conversion_id"`
		Legs         []struct {
			Level     int     `json:"level"`
			AmountRub float64 `json:"amount_rub"`
		} `json:"legs"`
	}
	if err := env.UnmarshalData(&data); err != nil {
		t.Fatalf("data не разобралась: %v", err)
	}
	// ⚠ Ног МАССИВ: клиент, ожидающий одну, потеряет остальные колена молча.
	if len(data.Legs) != 2 || data.Legs[1].Level != 2 {
		t.Errorf("ноги каскада разобраны не то: %+v", data.Legs)
	}
}

func TestParseRefusesEnvelopeWithoutEventID(t *testing.T) {
	// Тело подписано верно, но опознать событие при повторе нечем.
	_, err := webhook.Parse([]byte(`{"event_type":"lead.created","data":{}}`))
	if !errors.Is(err, webhook.ErrMalformedEnvelope) {
		t.Fatalf("конверт без event_id принят: %v", err)
	}
}

func TestParseRefusesNonEnvelope(t *testing.T) {
	_, err := webhook.Parse([]byte(`не json вовсе`))
	if !errors.Is(err, webhook.ErrMalformedEnvelope) {
		t.Fatalf("мусор принят как конверт: %v", err)
	}
}

// ── повтор ───────────────────────────────────────────────────────────────────

// TestReplayPassesSignatureAndIsCaughtOnlyByDedup — то, ради чего ST-4 вообще
// существует.
//
// ⚠⚠ Вектор `federation_replay_same_event_id` несёт ЗАКОННУЮ свежую подпись и
// то же `event_id`, что и предыдущий случай. Проверка подписи его пропускает —
// и обязана пропускать. Значит защита от второй обработки держится ТОЛЬКО на
// дедупликации, и без неё интегратор начислил бы дважды, имея при этом
// полностью зелёную проверку подписи.
func TestReplayPassesSignatureAndIsCaughtOnlyByDedup(t *testing.T) {
	v := load(t)
	first := caseByName(t, v, "federation_conversion_cascaded")
	replay := caseByName(t, v, "federation_replay_same_event_id")

	// Оба конверта — об одном событии.
	envFirst, err := webhook.Parse([]byte(first.RawBody))
	if err != nil {
		t.Fatalf("первый конверт: %v", err)
	}
	envReplay, err := webhook.Parse([]byte(replay.RawBody))
	if err != nil {
		t.Fatalf("повтор: %v", err)
	}
	if envFirst.EventID != envReplay.EventID {
		t.Fatalf("векторы перестали быть повтором одного события: %q против %q — "+
			"на таких данных проверка дедупликации зелена по пустоте",
			envFirst.EventID, envReplay.EventID)
	}
	if first.RawBody == replay.RawBody {
		t.Fatal("тела совпали байт в байт — повтор перестал отличаться от первой доставки, " +
			"и проверка перестала мерить то, ради чего заведена")
	}

	// 1. Подпись повтор НЕ ловит. Это утверждение о защите, а не о тесте:
	//    сломайся оно — и дедупликация была бы не нужна.
	if !webhook.Verify(v.Secret, replay.Header, []byte(replay.RawBody),
		time.Unix(replay.Timestamp, 0), v.tolerance()) {
		t.Fatal("подпись повтора не сошлась — вектор собран неверно")
	}

	// 2. Ловит его дедупликация.
	seen := webhook.NewMemorySeen(time.Hour)
	r := webhook.Receiver{
		Secret:    v.Secret,
		Tolerance: v.tolerance(),
		Seen:      seen,
		Now:       func() time.Time { return time.Unix(first.Timestamp, 0) },
	}

	if _, err := r.Accept(ctx(), first.Header, []byte(first.RawBody)); err != nil {
		t.Fatalf("первая доставка отбита: %v", err)
	}

	r.Now = func() time.Time { return time.Unix(replay.Timestamp, 0) }
	_, err = r.Accept(ctx(), replay.Header, []byte(replay.RawBody))
	if !errors.Is(err, webhook.ErrDuplicate) {
		t.Fatalf("повтор принят как новое событие (%v) — интегратор начислил бы дважды", err)
	}
}

// TestWithoutSeenStoreReplayGoesThrough — та же мысль с другой стороны: без
// памяти повтор проходит. Проверка существует, чтобы «защита есть» не
// подтверждалось тестом, который был бы зелёным и без защиты.
func TestWithoutSeenStoreReplayGoesThrough(t *testing.T) {
	v := load(t)
	first := caseByName(t, v, "federation_conversion_cascaded")
	replay := caseByName(t, v, "federation_replay_same_event_id")

	r := webhook.Receiver{Secret: v.Secret, Tolerance: v.tolerance()}

	r.Now = func() time.Time { return time.Unix(first.Timestamp, 0) }
	if _, err := r.Accept(ctx(), first.Header, []byte(first.RawBody)); err != nil {
		t.Fatalf("первая доставка: %v", err)
	}
	r.Now = func() time.Time { return time.Unix(replay.Timestamp, 0) }
	if _, err := r.Accept(ctx(), replay.Header, []byte(replay.RawBody)); err != nil {
		t.Fatalf("без памяти повтор обязан пройти, иначе предыдущий тест "+
			"зеленел бы и без дедупликации: %v", err)
	}
}

// TestSignatureIsCheckedBeforeDedup — порядок шагов как защита.
//
// ⚠⚠ Проверь повтор ДО подписи — и кто угодно, зная адрес, забил бы память
// придуманными event_id. Настоящая доставка после этого пришла бы как
// «повтор», вы ответили бы 200, и событие пропало бы молча.
func TestSignatureIsCheckedBeforeDedup(t *testing.T) {
	v := load(t)
	c := caseByName(t, v, "federation_conversion_cascaded")

	seen := webhook.NewMemorySeen(time.Hour)
	r := webhook.Receiver{
		Secret:    v.Secret,
		Tolerance: v.tolerance(),
		Seen:      seen,
		Now:       func() time.Time { return time.Unix(c.Timestamp, 0) },
	}

	// Чужой шлёт то же тело с испорченной подписью.
	if _, err := r.Accept(ctx(), c.TamperedHeader, []byte(c.RawBody)); !errors.Is(err, webhook.ErrBadSignature) {
		t.Fatalf("подделанная подпись принята: %v", err)
	}

	// И память об этом event_id не узнала: настоящая доставка проходит.
	env, err := r.Accept(ctx(), c.Header, []byte(c.RawBody))
	if err != nil {
		t.Fatalf("настоящая доставка отбита после попытки подделки (%v) — "+
			"значит неподписанное тело успело отравить память обработанных", err)
	}
	if env.EventType != "conversion.cascaded" {
		t.Errorf("разобрано не то событие: %q", env.EventType)
	}
}

// TestStoreFailureIsNotTreatedAsFresh — отказ памяти не значит «новое».
func TestStoreFailureIsNotTreatedAsFresh(t *testing.T) {
	v := load(t)
	c := caseByName(t, v, "federation_lead_created")

	r := webhook.Receiver{
		Secret:    v.Secret,
		Tolerance: v.tolerance(),
		Seen:      brokenStore{},
		Now:       func() time.Time { return time.Unix(c.Timestamp, 0) },
	}
	_, err := r.Accept(ctx(), c.Header, []byte(c.RawBody))
	if err == nil {
		t.Fatal("отказ памяти прочитан как «событие новое» — повтор обработался бы " +
			"ровно тогда, когда база недоступна")
	}
	if errors.Is(err, webhook.ErrDuplicate) {
		t.Fatal("отказ памяти выдан за повтор — Recca перестала бы доставлять")
	}
}

type brokenStore struct{}

func (brokenStore) MarkSeen(context.Context, string) (bool, error) {
	return false, errors.New("база недоступна")
}

// ── память ───────────────────────────────────────────────────────────────────

func TestMemorySeenForgetsAfterTTL(t *testing.T) {
	seen := webhook.NewMemorySeen(50 * time.Millisecond)

	if fresh, _ := seen.MarkSeen(ctx(), "evt_1"); !fresh {
		t.Fatal("первое событие не признано новым")
	}
	if fresh, _ := seen.MarkSeen(ctx(), "evt_1"); fresh {
		t.Fatal("повтор признан новым")
	}
	time.Sleep(80 * time.Millisecond)
	if fresh, _ := seen.MarkSeen(ctx(), "evt_1"); !fresh {
		t.Error("после истечения срока событие обязано считаться новым: " +
			"память в процессе живёт ограниченно и обещает не больше этого")
	}
}

func TestMemorySeenIsConcurrencySafe(t *testing.T) {
	seen := webhook.NewMemorySeen(time.Hour)
	const n = 64

	// ⚠ Ретрай обгоняет первую попытку — это не редкость, а обычный случай.
	// Проверка отдельно от записи пропустила бы обе доставки.
	results := make(chan bool, n)
	for range n {
		go func() {
			fresh, _ := seen.MarkSeen(ctx(), "evt_race")
			results <- fresh
		}()
	}
	freshCount := 0
	for range n {
		if <-results {
			freshCount++
		}
	}
	if freshCount != 1 {
		t.Errorf("новым событие признано %d раз из %d — ждали ровно один", freshCount, n)
	}
}

// ── все векторы через приёмник ───────────────────────────────────────────────

func TestAcceptHandlesEveryVector(t *testing.T) {
	v := load(t)
	fed := 0
	for _, c := range v.Cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			r := webhook.Receiver{
				Secret:    v.Secret,
				Tolerance: v.tolerance(),
				Now:       func() time.Time { return time.Unix(c.Timestamp, 0) },
			}
			env, err := r.Accept(ctx(), c.Header, []byte(c.RawBody))

			// Один вектор намеренно не является конвертом — он про побайтовую
			// подпись тела не в канонической форме.
			if c.Name == "raw_body_signed_verbatim" {
				if !errors.Is(err, webhook.ErrMalformedEnvelope) {
					t.Fatalf("вектор без event_id принят как конверт: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("вектор не принят: %v\n  почему случай важен: %s", err, c.What)
			}
			var body map[string]json.RawMessage
			if err := json.Unmarshal([]byte(c.RawBody), &body); err != nil {
				t.Fatalf("тело вектора не разобралось: %v", err)
			}
			if env.EventType == "" {
				t.Error("тип события пуст")
			}
		})
		if len(c.Name) > 11 && c.Name[:11] == "federation_" {
			fed++
		}
	}

	// ⚠⚠ Знаменатель проверяется ЯВНО. До WV-0804 векторов было три, все
	// `cpa.*`, и клиент, разобравший их, считался проверенным — хотя девяти
	// событий федерации не видел ни разу. Пропади они снова, и этот тест
	// остался бы зелёным по пустоте, не заметив сужения.
	if fed < 3 {
		t.Errorf("векторов федерации %d, ждали не меньше 3 — "+
			"знаменатель проверки сузился до одних только событий CPA", fed)
	}
}
