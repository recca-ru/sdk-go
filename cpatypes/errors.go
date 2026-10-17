package cpatypes

import (
	"errors"
	"net/http"

	"go.recca.ru/sdk/reccatypes"
)

// Машинерия ошибок переехала в reccatypes: тип кода, APIError, ValidationError и
// реестр — одни на все двери Recca, и транспорт, общий у CPA и федерации, не
// может зависеть от словаря одной из них.
//
// ⚠ Здесь ПСЕВДОНИМЫ, а не вторые определения: значение то же самое, errors.Is и
// errors.As работают одинаково, как ни назови.
type (
	// ErrorCode — код отказа двери.
	ErrorCode = reccatypes.ErrorCode
	// APIError — отказ двери: код, сообщение, статус, request_id.
	APIError = reccatypes.APIError
	// ValidationError — отказ, который клиент дал сам, не обращаясь к двери.
	ValidationError = reccatypes.ValidationError
)

// Коды ОБЩЕЙ рамы — ключ, право, частота, сбой, негодный вход. Они приезжают с
// любой двери Recca, поэтому и живут в reccatypes; здесь только имена, чтобы
// импорт CPA оставался одним.
const (
	CodeMissingTenantScope = reccatypes.CodeMissingTenantScope
	CodeInsufficientScope  = reccatypes.CodeInsufficientScope
	CodeRateLimitExceeded  = reccatypes.CodeRateLimitExceeded
	CodeInternalError      = reccatypes.CodeInternalError
	CodeInvalidInput       = reccatypes.CodeInvalidInput
	CodeMalformedResponse  = reccatypes.CodeMalformedResponse
)

// Коды, которые дают ОБЕ двери Recca — CPA и федерация (WV-0804). Определение
// у них одно, в reccatypes, и это не вкусовщина: реестр ключуется кодом и
// ПАНИКУЕТ на второй регистрации с другим сентинелом. Имена здесь остались,
// чтобы существующий код, написанный до выноса, продолжал собираться без
// правок, — и значение у них то же самое, то есть errors.Is работает как
// работал. Изменился ровно один разряд: приставка в ТЕКСТЕ отказа стала
// «recca:» вместо «cpa:», потому что отказ принадлежит обеим дверям.
const (
	CodeReservedExternalID = reccatypes.CodeReservedExternalID
	CodeOfferNotFound      = reccatypes.CodeOfferNotFound
	CodeConversionNotFound = reccatypes.CodeConversionNotFound
	CodeReversalFailed     = reccatypes.CodeReversalFailed
)

var (
	ErrReservedExternalID = reccatypes.ErrReservedExternalID
	ErrOfferNotFound      = reccatypes.ErrOfferNotFound
	ErrConversionNotFound = reccatypes.ErrConversionNotFound
	ErrReversalFailed     = reccatypes.ErrReversalFailed
)

// Коды, которые знает ТОЛЬКО CPA-дверь — раздел 12 контракта.
const (
	// ── рама продукта ────────────────────────────────────────────────────
	CodeFeatureNotAvailable ErrorCode = "FEATURE_NOT_AVAILABLE"
	CodeProductPaused       ErrorCode = "PRODUCT_PAUSED"

	// ── вход ─────────────────────────────────────────────────────────────
	CodePIINotAccepted ErrorCode = "PII_NOT_ACCEPTED"

	// ── чего не нашли ────────────────────────────────────────────────────
	CodePartnerNotFound    ErrorCode = "PARTNER_NOT_FOUND"
	CodePartnerNotActive   ErrorCode = "PARTNER_NOT_ACTIVE"
	CodeAdvertiserNotFound ErrorCode = "ADVERTISER_NOT_FOUND"
	CodePayoutNotFound     ErrorCode = "PAYOUT_NOT_FOUND"

	// ── денежные отказы ──────────────────────────────────────────────────
	CodeUnknownGoal           ErrorCode = "UNKNOWN_GOAL"
	CodeGoalDisabled          ErrorCode = "GOAL_DISABLED"
	CodeWindowExpired         ErrorCode = "WINDOW_EXPIRED"
	CodeOccurredAtOutOfWindow ErrorCode = "OCCURRED_AT_OUT_OF_WINDOW"
	CodeAlreadyConverted      ErrorCode = "ALREADY_CONVERTED"
	CodeExternalIDConflict    ErrorCode = "EXTERNAL_ID_CONFLICT"
	CodeIllegalTransition     ErrorCode = "ILLEGAL_TRANSITION"
	CodeIdempotentMismatch    ErrorCode = "IDEMPOTENT_MISMATCH"
	CodeBelowMinPayout        ErrorCode = "BELOW_MIN_PAYOUT"
	CodeInsufficientBalance   ErrorCode = "INSUFFICIENT_BALANCE"
)

// Сентинелы общей рамы — те же значения, что в reccatypes. Переобъявлять их
// своими нельзя: два сентинела на один код означали бы, что errors.Is отвечает
// по-разному на отказ, который дала одна и та же проверка платформы.
var (
	ErrMissingTenantScope = reccatypes.ErrMissingTenantScope
	ErrInsufficientScope  = reccatypes.ErrInsufficientScope
	ErrRateLimited        = reccatypes.ErrRateLimited
	ErrInternal           = reccatypes.ErrInternal
	ErrInvalidInput       = reccatypes.ErrInvalidInput
	ErrMalformedResponse  = reccatypes.ErrMalformedResponse
	// ErrValidation — общий признак отказа, который клиент дал САМ, не
	// обращаясь к двери.
	ErrValidation = reccatypes.ErrValidation
)

// Сентинелы CPA-двери. У каждого кода свой — это и есть обещанный способ
// ветвиться через errors.Is, не сравнивая строки.
var (
	ErrFeatureNotAvailable = errors.New("cpa: продукт «CPA-сеть» не подключён")
	ErrProductPaused       = errors.New("cpa: продукт «CPA-сеть» приостановлен")

	ErrPIINotAccepted = errors.New("cpa: в свободном поле распознано персональное данное")

	ErrPartnerNotFound    = errors.New("cpa: партнёра с таким external_id в сети нет")
	ErrPartnerNotActive   = errors.New("cpa: партнёр сейчас работать не может")
	ErrAdvertiserNotFound = errors.New("cpa: рекламодателя с таким advertiser_id в сети нет")
	ErrPayoutNotFound     = errors.New("cpa: заявки на выплату с таким идентификатором нет")

	ErrUnknownGoal           = errors.New("cpa: у оффера нет такой цели")
	ErrGoalDisabled          = errors.New("cpa: цель выключена оператором сети — новые конверсии по ней не принимаются")
	ErrWindowExpired         = errors.New("cpa: окно атрибуции оффера истекло")
	ErrOccurredAtOutOfWindow = errors.New("cpa: время события вне окна приёма — конверсии принимаются не глубже 90 дней")
	ErrAlreadyConverted      = errors.New("cpa: по этому переходу уже засчитана конверсия на ту же цель")
	ErrExternalIDConflict    = errors.New("cpa: external_id занят в сети другим оффером по той же цели")
	ErrIllegalTransition     = errors.New("cpa: такой переход статуса невозможен")
	ErrIdempotentMismatch    = errors.New("cpa: операция с этим ключом идемпотентности была с другими параметрами")
	ErrBelowMinPayout        = errors.New("cpa: сумма меньше минимальной выплаты сети")
	ErrInsufficientBalance   = errors.New("cpa: доступного остатка партнёра не хватает")
)

// surface — приставка в тексте ошибок этой двери и признак отбора в реестре.
const surface = "cpa"

// info — одна горловина сборки записи реестра.
//
// ⚠ Поля названы ЗДЕСЬ и только здесь, а таблица ниже остаётся таблицей.
// Безключевой литерал через границу пакета `go vet` отбивает справедливо:
// допиши кто-нибудь пятое поле в reccatypes.CodeInfo — и девятнадцать записей
// разъехались бы МОЛЧА, подставив чужие значения по позиции.
func info(status int, sentinel error, retryable bool) reccatypes.CodeInfo {
	return reccatypes.CodeInfo{
		Surface:   surface,
		HTTP:      status,
		Sentinel:  sentinel,
		Retryable: retryable,
	}
}

func init() {
	reccatypes.Register(map[ErrorCode]reccatypes.CodeInfo{
		CodeFeatureNotAvailable: info(http.StatusForbidden, ErrFeatureNotAvailable, false),
		CodeProductPaused:       info(http.StatusForbidden, ErrProductPaused, false),

		CodePIINotAccepted: info(http.StatusBadRequest, ErrPIINotAccepted, false),

		CodePartnerNotFound:    info(http.StatusNotFound, ErrPartnerNotFound, false),
		CodePartnerNotActive:   info(http.StatusConflict, ErrPartnerNotActive, false),
		CodeAdvertiserNotFound: info(http.StatusNotFound, ErrAdvertiserNotFound, false),
		CodePayoutNotFound:     info(http.StatusNotFound, ErrPayoutNotFound, false),

		CodeUnknownGoal:           info(http.StatusBadRequest, ErrUnknownGoal, false),
		CodeGoalDisabled:          info(http.StatusConflict, ErrGoalDisabled, false),
		CodeWindowExpired:         info(http.StatusConflict, ErrWindowExpired, false),
		CodeOccurredAtOutOfWindow: info(http.StatusBadRequest, ErrOccurredAtOutOfWindow, false),
		CodeAlreadyConverted:      info(http.StatusConflict, ErrAlreadyConverted, false),
		CodeExternalIDConflict:    info(http.StatusConflict, ErrExternalIDConflict, false),
		CodeIllegalTransition:     info(http.StatusConflict, ErrIllegalTransition, false),
		CodeIdempotentMismatch:    info(http.StatusConflict, ErrIdempotentMismatch, false),
		CodeBelowMinPayout:        info(http.StatusBadRequest, ErrBelowMinPayout, false),
		CodeInsufficientBalance:   info(http.StatusConflict, ErrInsufficientBalance, false),
	})
}

// Codes перечисляет коды, известные клиенту CPA, в устойчивом порядке: свои
// плюс коды общей рамы, которые приезжают с любой двери.
//
// ⚠ Отбор по поверхности, а не «все, что зарегистрированы»: программа, которая
// импортировала и клиент федерации, иначе увидела бы здесь чужие коды — и
// сверка с контрактом CPA покраснела бы по ложной причине.
func Codes() []ErrorCode { return reccatypes.CodesOfSurface(surface) }

// HTTPStatusFor называет статус, с которым код приезжает по контракту. Второе
// значение — знаком ли код клиенту вообще: новый код на новом условии считается
// совместимой правкой (раздел 15), и клиент обязан его пережить.
func HTTPStatusFor(code ErrorCode) (int, bool) { return reccatypes.HTTPStatusFor(code) }

// SentinelFor возвращает сентинел кода или nil, если код клиенту неизвестен.
func SentinelFor(code ErrorCode) error { return reccatypes.SentinelFor(code) }

// Retryable отвечает, стоит ли повторять запрос, получивший этот отказ.
//
// ⚠⚠ Это утверждение об ОТКАЗЕ, а не разрешение повторять ОПЕРАЦИЮ. Повтор
// делает транспорт и только для запросов, объявленных идемпотентными: повтор
// неидемпотентного POST оплатил бы конверсию дважды.
func Retryable(code ErrorCode, status int) bool { return reccatypes.Retryable(code, status) }

// invalid — короткая форма для проверок пакета.
func invalid(field, reason string) error { return reccatypes.Invalid(field, reason) }
