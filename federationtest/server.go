// Package federationtest — стенд для тестирования интеграции с федеративным
// API Recca (`/federation/v1/*`).
//
// # Что этот стенд доказывает и чего он НЕ доказывает
//
// ⚠⚠ Стенд СТРУКТУРНЫЙ. Он отвечает на одиннадцать путей конвертом контракта и
// фикстурами из testdata/fixtures — и этим доказывает, что ваш код собирает
// верные запросы и разбирает верные ответы. **Поведение платформы он не
// воспроизводит и не обещает воспроизводить ни строкой.** Сколько заплатит
// каскад, кого найдёт сведение личности, что случится с деревом при отзыве
// ребра — доказывает песочница api-sandbox.recca.ru и набор Conformance против
// неё, а не этот файл.
//
// Граница названа прямо, потому что стенд, который «почти как настоящий»,
// опаснее отсутствия стенда: зелёный прогон против него читается как готовность
// к бою.
//
// # Три правила, которые стенд всё-таки исполняет
//
// Идемпотентность — единственная логика, кроме отдачи фикстур, и она здесь
// потому, что у федерации идемпотентность живёт в ТЕЛЕ, а не в заголовке.
// Проверить, что клиент шлёт различитель и правильно читает ответ на повтор,
// иначе нечем:
//
//   - повтор конверсии с тем же `external_id` возвращает ТУ ЖЕ строку,
//     `idempotent: true` и 200 вместо 201;
//   - повтор заявки с тем же `external_id` — то же самое;
//   - повтор чеканки ссылки на ту же пару «человек ⇆ оффер» — ту же ссылку и
//     200 вместо 201.
//
// ⚠ Заголовка `Idempotency-Key` стенд НЕ понимает намеренно, ровно как живая
// дверь: пришли его клиент — и стенд промолчал бы, а прогон остался бы
// зелёным. Стенд, прощающий то, чего дверь не прощает, хуже отсутствия стенда,
// поэтому есть отдельная проверка AssertNoIdempotencyHeader.
package federationtest

import (
	"embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"go.recca.ru/sdk/federation"
)

// fixtures — тела ответов. Файлами, а не литералами в Go: правка контракта
// меняет один файл, и видно её в дифе как данные, а не как код.
//
//go:embed testdata/fixtures/*.json
var fixtures embed.FS

// apiPrefix — версионированный префикс, который подставляет транспорт клиента.
const apiPrefix = "/federation/v1"

// Request — обращение, которое увидел стенд.
type Request struct {
	Method string
	// Path — путь ОТ версии двери: "/users/upsert".
	//
	// ⚠⚠ Он ДЕКОДИРОВАН: `net/http` раскрывает проценты, прежде чем отдать
	// путь обработчику. Значит по нему нельзя проверить, экранировал ли клиент
	// подстановку: «order%2F17» и «order/17» выглядят здесь ОДИНАКОВО, и
	// проверка экранирования на этом поле была бы зелёной на дыре. Для неё
	// есть EscapedPath.
	Path string
	// EscapedPath — путь так, как он ушёл по сети, вместе с процентами.
	EscapedPath string
	Query       url.Values
	Header      http.Header
	Body        []byte
}

// Server — стенд на httptest.
type Server struct {
	srv *httptest.Server

	mu       sync.Mutex
	requests []Request

	// pending — заготовленный отказ. held решает, снимается ли он после первого
	// же запроса (PrepareError) или держится до ClearError (HoldError).
	pending *federation.ErrorCode
	held    bool

	// Память идемпотентности; см. заголовок пакета.
	conversions map[string]json.RawMessage
	leads       map[string]json.RawMessage
	links       map[string]json.RawMessage

	// repeatStatus — код ответа идемпотентного повтора; 0 означает штатные 200.
	//
	// ⚠⚠ Не экспортируется и наружу не выйдет. Читатель у неё ровно один —
	// внутренние тесты, доказывающие, что Conformance КРАСНЕЕТ на повторе,
	// ответившем 201 вместо 200. Без такой проверки набор был бы утверждением о
	// себе самом: зелёный он и на верном поведении, и на сломанном, а отличить
	// нельзя. Дать ручку публично значило бы предложить интегратору способ
	// сделать свой прогон зелёным по неправильной причине.
	repeatStatus int

	// breakIdempotency заставляет стенд отвечать на повтор как на НОВУЮ
	// строку: без признака `idempotent`. Тот же единственный читатель — тест,
	// доказывающий, что набор приёмки это замечает. Наружу не выходит по той
	// же причине: это способ сделать чужой прогон зелёным по неправильной
	// причине.
	breakIdempotency bool

	// tamper — намеренная порча тела ответа, по той же причине и с тем же
	// ограничением.
	tamper func(path string, data json.RawMessage) json.RawMessage

	// environment подменяет среду в ответе /self. nil — фикстура, то есть
	// «sandbox». Читатель тот же: тест, доказывающий краснóту набора.
	//
	// ⚠⚠ УКАЗАТЕЛЬ, а не строка, и это не педантизм: у строки пустое значение
	// означало бы одновременно «не подменяли» и «подменили на пустое», а
	// второе — ровно тот случай, который обязан ловить набор (дверь старше
	// WV-0804 поля не шлёт). На строке мутация «пропускать неназванную среду»
	// осталась ЗЕЛЁНОЙ: тест вынужденно подставлял пробел и проверял не тот
	// случай. Найдено мутационным прогоном 28.09.2026, а не рассуждением.
	environment *string

	// dropEnvironment убирает поле из ответа ЦЕЛИКОМ — так отвечает дверь,
	// которая о среде не знает. Отдельно от пустой строки, потому что «поля
	// нет» и «поле пустое» приезжают по сети по-разному, а Go оба разбирает в
	// "" — и проверить, что набор переживает оба, можно только послав оба.
	dropEnvironment bool
}

// setEnvironment — только для внутренних тестов пакета: подменить среду,
// которую называет дверь.
//
// ⚠⚠ Не экспортируется наружу намеренно. Дай интегратору сказать стенду «ты
// песочница» — и он сможет сделать свой приёмочный прогон зелёным, обойдя
// ровно ту проверку, ради которой набор написан.
func (s *Server) setEnvironment(env string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.environment = &env
	s.dropEnvironment = false
}

// dropEnvironmentField заставляет дверь не называть среду вовсе.
func (s *Server) dropEnvironmentField() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.environment = nil
	s.dropEnvironment = true
}

// New поднимает стенд и гасит его по окончании теста.
func New(t *testing.T) *Server {
	t.Helper()
	s := &Server{
		conversions: map[string]json.RawMessage{},
		leads:       map[string]json.RawMessage{},
		links:       map[string]json.RawMessage{},
	}
	s.srv = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.srv.Close)
	return s
}

// URL — адрес стенда для ClientOptions.BaseURL.
func (s *Server) URL() string { return s.srv.URL }

// Client — клиент, настроенный на стенд.
func (s *Server) Client(t *testing.T) *federation.Client {
	t.Helper()
	c, err := federation.New("rtk_test_federationtest", &federation.ClientOptions{
		BaseURL: s.srv.URL,
		// ⚠ Повторов НЕТ по умолчанию: иначе тест, проверяющий число обращений,
		// считал бы попытки транспорта, а не вызовы метода.
		MaxRetries: ptr(0),
	})
	if err != nil {
		t.Fatalf("federationtest: клиент не собрался: %v", err)
	}
	return c
}

func ptr[T any](v T) *T { return &v }

// Requests — все обращения, которые видел стенд, в порядке поступления.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Request, len(s.requests))
	copy(out, s.requests)
	return out
}

// Last — последнее обращение. Второе значение — было ли оно вообще.
func (s *Server) Last() (Request, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.requests) == 0 {
		return Request{}, false
	}
	return s.requests[len(s.requests)-1], true
}

// Reset забывает обращения и память идемпотентности.
func (s *Server) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = nil
	s.conversions = map[string]json.RawMessage{}
	s.leads = map[string]json.RawMessage{}
	s.links = map[string]json.RawMessage{}
	s.pending = nil
	s.held = false
}

// PrepareError заготавливает отказ на ОДИН следующий запрос.
func (s *Server) PrepareError(code federation.ErrorCode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = &code
	s.held = false
}

// HoldError заготавливает отказ до ClearError: нужен там, где клиент повторяет
// запрос и одноразовая заготовка снялась бы первой же попыткой.
func (s *Server) HoldError(code federation.ErrorCode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = &code
	s.held = true
}

// ClearError снимает удержанный отказ.
func (s *Server) ClearError() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = nil
	s.held = false
}

// AssertNoIdempotencyHeader проверяет, что клиент НЕ слал `Idempotency-Key`.
//
// ⚠⚠ Проверка нужна потому, что её отсутствие невидимо: живая дверь федерации
// такого заголовка не читает и на лишний не жалуется, стенд тоже. То есть
// клиент, шлющий его «на всякий случай», остаётся зелёным везде — и при этом
// обещает своему потребителю защиту, которой нет. Единственный способ узнать
// правду — спросить прямо.
func (s *Server) AssertNoIdempotencyHeader(t *testing.T) {
	t.Helper()
	for _, r := range s.Requests() {
		if v := r.Header.Get("Idempotency-Key"); v != "" {
			t.Errorf("federationtest: %s %s несёт Idempotency-Key %q — "+
				"дверь федерации его НЕ читает, и слать его значит обещать идемпотентность, которой нет",
				r.Method, r.Path, v)
		}
	}
}

// handle записывает обращение, отдаёт заготовленный отказ либо маршрутизирует.
func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	body := readAll(r)
	path := strings.TrimPrefix(r.URL.Path, apiPrefix)

	s.mu.Lock()
	s.requests = append(s.requests, Request{
		Method:      r.Method,
		Path:        path,
		EscapedPath: strings.TrimPrefix(r.URL.EscapedPath(), apiPrefix),
		Query:       r.URL.Query(),
		Header:      r.Header.Clone(),
		Body:        body,
	})
	requestID := "req_federationtest_" + strconv.Itoa(len(s.requests))
	pending := s.pending
	// ⚠ Одноразовая заготовка снимается ЗДЕСЬ, до ответа: иначе повторный вызов
	// получил бы тот же отказ, и «ровно один раз» превратилось бы в «до отмены».
	if !s.held {
		s.pending = nil
	}
	s.mu.Unlock()

	if pending != nil {
		s.writeError(w, requestID, *pending, "federationtest: отказ заготовлен PrepareError")
		return
	}

	s.route(w, r, path, body, requestID)
}
