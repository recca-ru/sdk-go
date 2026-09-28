package reccatypes

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"
)

// ErrorCode — код отказа двери. Ветвиться нужно по нему, а не по тексту
// сообщения: текст — диагностика для человека, он меняется без предупреждения и
// частью контракта не является.
type ErrorCode string

// ── Коды ОБЩЕЙ рамы ──────────────────────────────────────────────────────────
//
// Их ставит не дверь, а рама перед ней: `tenantScopeMiddleware` (ключ и право),
// ограничитель частоты, общий обработчик сбоя. Поверхность на них не влияет,
// поэтому и сентинел у них ОДИН: два сентинела на один факт заставили бы
// потребителя спрашивать `errors.Is` дважды, и однажды он спросил бы не то.
const (
	CodeMissingTenantScope ErrorCode = "MISSING_TENANT_SCOPE"
	CodeInsufficientScope  ErrorCode = "INSUFFICIENT_SCOPE"
	CodeRateLimitExceeded  ErrorCode = "RATE_LIMIT_EXCEEDED"
	CodeInternalError      ErrorCode = "INTERNAL_ERROR"
	CodeInvalidInput       ErrorCode = "INVALID_INPUT"

	// CodeMalformedResponse ставит САМ клиент, когда дверь ответила не по
	// контракту: 2xx с success:false, отказ без конверта. Дверь такого кода не
	// присылает — потому его и нет ни в одном реестре Recca.
	//
	// ⚠ Подставлять сюда статус нельзя: код вида HTTP_200 в журнале потребителя
	// выглядел бы как подтверждение успеха.
	CodeMalformedResponse ErrorCode = "MALFORMED_RESPONSE"
)

// Сентинелы общей рамы. Приставка «recca», а не имя поверхности: отказ
// принадлежит платформе целиком, и притворяться, будто его дала конкретная
// дверь, значило бы отправить интегратора искать причину не там.
var (
	ErrMissingTenantScope = errors.New("recca: ключ арендатора не предъявлен или отозван")
	ErrInsufficientScope  = errors.New("recca: у ключа нет права на этот метод")
	ErrRateLimited        = errors.New("recca: исчерпан бюджет запросов ключа")
	ErrInternal           = errors.New("recca: сбой на стороне Recca")
	ErrInvalidInput       = errors.New("recca: тело или параметры не прошли проверку двери")
	ErrMalformedResponse  = errors.New("recca: дверь ответила не по контракту")
)

// ── Коды, которые дают ОБЕ двери ─────────────────────────────────────────────
//
// ⚠⚠ Здесь они не из аккуратности, а потому что реестр иначе ПАНИКУЕТ: он один
// на модуль и ключуется кодом, и вторая регистрация того же кода с другим
// сентинелом — объявленный отказ (см. Register). Четыре кода ниже CPA-дверь и
// федерация дают ОБЕ, и означают они одно и то же — `RESERVED_EXTERNAL_ID` у
// них даже считает один и тот же `conversion-external-id.ts`.
//
// ⚠ Поэтому же Surface у них «recca», а не имя одной из дверей: выбрать одну
// значило бы, что отказ ДРУГОЙ печатается чужой приставкой, причём какой именно
// — решал бы порядок загрузки пакетов. CodesOfSurface складывает «recca» к
// любой поверхности, так что из перечня ни у CPA, ни у федерации они не
// выпадают.
//
// ⚠ Дверь федерации называет их своими статусами, и они СОВПАЛИ с CPA по всем
// четырём (замер реестра дверей 28.09.2026: 400 · 404 · 404 · 500). Разойдись
// хоть один — код пришлось бы разводить, а не объединять.
const (
	CodeReservedExternalID ErrorCode = "RESERVED_EXTERNAL_ID"
	CodeOfferNotFound      ErrorCode = "OFFER_NOT_FOUND"
	CodeConversionNotFound ErrorCode = "CONVERSION_NOT_FOUND"
	CodeReversalFailed     ErrorCode = "REVERSAL_FAILED"
)

var (
	ErrReservedExternalID = errors.New("recca: external_id из зарезервированного пространства платформы")
	ErrOfferNotFound      = errors.New("recca: оффера с таким идентификатором нет")
	ErrConversionNotFound = errors.New("recca: конверсии с таким идентификатором нет")
	ErrReversalFailed     = errors.New("recca: возврат начался, но не завершился")
)

// CodeInfo — что клиент знает о коде: чья это поверхность, с каким HTTP код
// приезжает и переживает ли повтор сам по себе.
//
// ⚠ Повторяемость КОДА и повторяемость СТАТУСА — независимые признаки, и оба
// нужны: код может быть новым (его ещё нет в реестре), а 5xx и 429 говорят о
// сбое сами. Сводить их в один признак значило бы потерять один из двух.
type CodeInfo struct {
	// Surface — имя поверхности для текста ошибки: «cpa», «federation»,
	// «recca» у общей рамы. Не контракт, а приставка, по которой человек в
	// тикете понимает, какую дверь звали.
	Surface string
	// HTTP — статус, с которым код приезжает по контракту. Ноль означает
	// «своего статуса нет» — так у кода, который ставит сам клиент.
	HTTP int
	// Sentinel — значение для errors.Is.
	Sentinel error
	// Retryable — переживает ли ЭТОТ ОТКАЗ повтор.
	Retryable bool
}

// registry — один реестр на весь модуль. Поверхности дописывают в него свои
// коды из init(); общая рама зарегистрирована ниже.
//
// ⚠⚠ Реестр ОДИН, а не по одному на поверхность, и это решение. Наличие
// нескольких реестров означало бы, что APIError обязан знать, в каком из них
// искать свой сентинел, — то есть носить ссылку на реестр полем. А тогда
// `&APIError{Code: …}`, собранный литералом (так его собирают тесты и так его
// естественно собрать потребителю), молча перестал бы отвечать на errors.Is.
var registry = struct {
	sync.RWMutex
	codes map[ErrorCode]CodeInfo
}{codes: map[ErrorCode]CodeInfo{}}

// Register вносит коды поверхности в общий реестр.
//
// ⚠⚠ Повторная регистрация того же кода с ДРУГИМ сентинелом — паника при
// загрузке пакета, а не тихая победа первого. Два сентинела на один код значат,
// что две поверхности разошлись в том, что этот код означает, и узнать об этом
// на запуске несравнимо дешевле, чем по ветвлению, которое однажды не сработало
// у интегратора. Повторная регистрация ОДНОГО И ТОГО ЖЕ — не ошибка: так
// поверхность может назвать код общей рамы в своей таблице, не расходясь с ней.
func Register(codes map[ErrorCode]CodeInfo) {
	registry.Lock()
	defer registry.Unlock()
	for code, info := range codes {
		if was, ok := registry.codes[code]; ok && was.Sentinel != info.Sentinel {
			panic(fmt.Sprintf(
				"reccatypes: код %s регистрируется дважды с разными сентинелами (%q и %q) — "+
					"две поверхности разошлись в том, что он означает",
				code, was.Surface, info.Surface,
			))
		}
		registry.codes[code] = info
	}
}

// frameInfo — запись общей рамы. Одна горловина, где названы поля.
//
// ⚠ Литерал с ключами, а не по позиции, и здесь тоже — хотя `go vet` внутри
// своего пакета об этом не напомнит. Довод тот же, что у поверхностей: допиши
// кто-нибудь пятое поле в CodeInfo, и записи разъехались бы молча, подставив
// чужое значение по позиции.
func frameInfo(status int, sentinel error, retryable bool) CodeInfo {
	return CodeInfo{Surface: "recca", HTTP: status, Sentinel: sentinel, Retryable: retryable}
}

func init() {
	Register(map[ErrorCode]CodeInfo{
		CodeMissingTenantScope: frameInfo(http.StatusUnauthorized, ErrMissingTenantScope, false),
		CodeInsufficientScope:  frameInfo(http.StatusForbidden, ErrInsufficientScope, false),
		CodeRateLimitExceeded:  frameInfo(http.StatusTooManyRequests, ErrRateLimited, true),
		CodeInternalError:      frameInfo(http.StatusInternalServerError, ErrInternal, true),
		CodeInvalidInput:       frameInfo(http.StatusBadRequest, ErrInvalidInput, false),
		// ⚠ HTTP = 0: код ставит САМ клиент, своего статуса у него нет.
		CodeMalformedResponse: frameInfo(0, ErrMalformedResponse, false),

		// ── коды, живущие на ОБЕИХ дверях ────────────────────────────
		CodeReservedExternalID: frameInfo(http.StatusBadRequest, ErrReservedExternalID, false),
		CodeOfferNotFound:      frameInfo(http.StatusNotFound, ErrOfferNotFound, false),
		CodeConversionNotFound: frameInfo(http.StatusNotFound, ErrConversionNotFound, false),
		CodeReversalFailed:     frameInfo(http.StatusInternalServerError, ErrReversalFailed, true),
	})
}

// lookup — чтение реестра под общей блокировкой.
func lookup(code ErrorCode) (CodeInfo, bool) {
	registry.RLock()
	defer registry.RUnlock()
	info, ok := registry.codes[code]
	return info, ok
}

// Codes перечисляет ВСЕ коды, известные модулю, в устойчивом порядке.
//
// ⚠ Поверхности нужен её СОБСТВЕННЫЙ перечень, а не этот: здесь лежат коды всех
// дверей сразу. Отбор — CodesOfSurface.
func Codes() []ErrorCode {
	registry.RLock()
	defer registry.RUnlock()
	out := make([]ErrorCode, 0, len(registry.codes))
	for code := range registry.codes {
		out = append(out, code)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// CodesOfSurface перечисляет коды одной поверхности вместе с кодами общей рамы:
// рама отказывает на каждой двери, и перечень без неё был бы неполон.
func CodesOfSurface(surface string) []ErrorCode {
	registry.RLock()
	defer registry.RUnlock()
	out := make([]ErrorCode, 0, len(registry.codes))
	for code, info := range registry.codes {
		if info.Surface == surface || info.Surface == "recca" {
			out = append(out, code)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// HTTPStatusFor называет статус, с которым код приезжает по контракту. Второе
// значение — знаком ли код клиенту вообще: новый код на новом условии считается
// совместимой правкой, и клиент обязан его пережить.
func HTTPStatusFor(code ErrorCode) (int, bool) {
	info, ok := lookup(code)
	return info.HTTP, ok
}

// SentinelFor возвращает сентинел кода или nil, если код клиенту неизвестен.
func SentinelFor(code ErrorCode) error {
	if info, ok := lookup(code); ok {
		return info.Sentinel
	}
	return nil
}

// SurfaceFor называет поверхность кода; для неизвестного — «recca».
func SurfaceFor(code ErrorCode) string {
	if info, ok := lookup(code); ok && info.Surface != "" {
		return info.Surface
	}
	return "recca"
}

// Retryable отвечает, стоит ли повторять запрос, получивший этот отказ.
//
// ⚠⚠ Это утверждение об ОТКАЗЕ, а не разрешение повторять ОПЕРАЦИЮ. Повтор
// делает транспорт и только для запросов, объявленных идемпотентными: повтор
// неидемпотентного POST оплатил бы конверсию дважды.
func Retryable(code ErrorCode, status int) bool {
	if status >= http.StatusInternalServerError || status == http.StatusTooManyRequests {
		return true
	}
	if info, ok := lookup(code); ok {
		return info.Retryable
	}
	return false
}

// APIError — отказ двери: код, сообщение, статус и идентификатор запроса.
//
// Идентификатор — поле, а не строка в журнале SDK: по нему нас просят искать
// цепочку на своей стороне, и потребителю он нужен в тикете.
type APIError struct {
	// Code — код контракта; сравнивать его лучше через errors.Is с сентинелом.
	Code ErrorCode
	// Message — диагностика для человека. Частью контракта не является.
	Message string
	// HTTPStatus — статус ответа, с которым отказ приехал.
	HTTPStatus int
	// RequestID — meta.request_id либо заголовок X-Request-Id.
	RequestID string
	// Details — поле «details» конверта, как прислала дверь: на отказах про
	// вход оно называет поле поимённо.
	Details json.RawMessage
	// Retryable — переживает ли этот отказ повтор (см. Retryable).
	Retryable bool
}

func (e *APIError) Error() string {
	surface := SurfaceFor(e.Code)
	if e.RequestID != "" {
		return fmt.Sprintf("%s: %s (HTTP %d, request_id %s): %s",
			surface, e.Code, e.HTTPStatus, e.RequestID, e.Message)
	}
	return fmt.Sprintf("%s: %s (HTTP %d): %s", surface, e.Code, e.HTTPStatus, e.Message)
}

// Unwrap выдаёт сентинел кода, чтобы errors.Is находил его без сравнения строк.
// Для неизвестного кода — nil: иначе ошибка совпала бы с чужим сентинелом и
// потребитель увидел бы не ту причину.
func (e *APIError) Unwrap() error { return SentinelFor(e.Code) }

// ErrValidation — общий признак отказа, который клиент дал САМ, не обращаясь к
// двери. Отличать его от ответа Recca необходимо: иначе интегратор искал бы
// свой request_id в наших журналах.
var ErrValidation = errors.New("recca: запрос не прошёл проверку до отправки")

// ValidationError называет поле и причину.
type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("recca: поле %s не прошло проверку до отправки: %s", e.Field, e.Reason)
}

func (e *ValidationError) Unwrap() error { return ErrValidation }

// Invalid — короткая форма отказа до сети.
func Invalid(field, reason string) error {
	return &ValidationError{Field: field, Reason: reason}
}
