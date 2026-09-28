package federation

import "go.recca.ru/sdk/federationtypes"

// Ошибки двери переэкспортированы из federationtypes, чтобы квикстарту хватало
// одного импорта. Значения те же самые — это алиасы, а не вторые определения:
// errors.Is и errors.As работают одинаково, как ни назови.
//
// ⚠ Добавили код в federationtypes — добавьте строку и сюда. Компилятор об
// этом не напомнит: отсутствие имени здесь ничего не ломает, оно просто делает
// код недоступным под коротким именем.
//
// ⚠ Типы ErrorCode, APIError и ValidationError объявлены в types.go — тут их
// повторять нельзя, пакет один.

const (
	// ── рама ─────────────────────────────────────────────────────────────
	CodeMissingTenantScope = federationtypes.CodeMissingTenantScope
	CodeInsufficientScope  = federationtypes.CodeInsufficientScope
	CodeRateLimitExceeded  = federationtypes.CodeRateLimitExceeded
	CodeInternalError      = federationtypes.CodeInternalError
	CodeInvalidInput       = federationtypes.CodeInvalidInput
	CodeMalformedResponse  = federationtypes.CodeMalformedResponse

	// ── ключ и право ─────────────────────────────────────────────────────
	CodeMissingTenantKey = federationtypes.CodeMissingTenantKey
	CodeInvalidTenantKey = federationtypes.CodeInvalidTenantKey
	CodeIPNotAllowed     = federationtypes.CodeIPNotAllowed
	CodeUnauthorized     = federationtypes.CodeUnauthorized

	// ── доступ от имени человека ─────────────────────────────────────────
	CodeMissingToken = federationtypes.CodeMissingToken
	CodeInvalidToken = federationtypes.CodeInvalidToken
	CodeGrantRevoked = federationtypes.CodeGrantRevoked

	// ── вход ─────────────────────────────────────────────────────────────
	CodeValidationError    = federationtypes.CodeValidationError
	CodeInvalidTemplate    = federationtypes.CodeInvalidTemplate
	CodeReservedExternalID = federationtypes.CodeReservedExternalID

	// ── членство (два имени у одного факта) ──────────────────────────────
	CodeNotAMember            = federationtypes.CodeNotAMember
	CodeUserNotMemberOfTenant = federationtypes.CodeUserNotMemberOfTenant

	// ── чего не нашли ────────────────────────────────────────────────────
	CodeNotFound          = federationtypes.CodeNotFound
	CodeOfferNotFound     = federationtypes.CodeOfferNotFound
	CodeProgramNotFound   = federationtypes.CodeProgramNotFound
	CodeNoEligibleProgram = federationtypes.CodeNoEligibleProgram

	// ── состояние программы промокодов ───────────────────────────────────
	CodeProgramInactive     = federationtypes.CodeProgramInactive
	CodeCodeIssuedByOwner   = federationtypes.CodeCodeIssuedByOwner
	CodeCodeNotAvailableYet = federationtypes.CodeCodeNotAvailableYet

	// ── состояние предмета ───────────────────────────────────────────────
	CodeConflict = federationtypes.CodeConflict

	// ── привлечение ──────────────────────────────────────────────────────
	CodeInvalidSignedRef = federationtypes.CodeInvalidSignedRef
	CodeTenantMismatch   = federationtypes.CodeTenantMismatch
	CodeBadgeNotActive   = federationtypes.CodeBadgeNotActive

	// ── деньги ───────────────────────────────────────────────────────────
	CodeInvalidCommissionConfig = federationtypes.CodeInvalidCommissionConfig
	CodeRateUnknown             = federationtypes.CodeRateUnknown
	CodeCascadeNotAvailable     = federationtypes.CodeCascadeNotAvailable
	CodeCascadeFailed           = federationtypes.CodeCascadeFailed
	CodeConversionNotFound      = federationtypes.CodeConversionNotFound
	CodeNotReversible           = federationtypes.CodeNotReversible
	CodeAmountExceedsOriginal   = federationtypes.CodeAmountExceedsOriginal
	CodeReversalFailed          = federationtypes.CodeReversalFailed

	// ── личность ─────────────────────────────────────────────────────────
	CodeEmailBelongsToDifferentUser = federationtypes.CodeEmailBelongsToDifferentUser
	CodePhoneBelongsToDifferentUser = federationtypes.CodePhoneBelongsToDifferentUser
)

var (
	ErrMissingTenantScope = federationtypes.ErrMissingTenantScope
	ErrInsufficientScope  = federationtypes.ErrInsufficientScope
	ErrRateLimited        = federationtypes.ErrRateLimited
	ErrInternal           = federationtypes.ErrInternal
	ErrInvalidInput       = federationtypes.ErrInvalidInput
	ErrMalformedResponse  = federationtypes.ErrMalformedResponse
	ErrValidation         = federationtypes.ErrValidation
	ErrIncompleteData     = federationtypes.ErrIncompleteData

	ErrMissingTenantKey = federationtypes.ErrMissingTenantKey
	ErrInvalidTenantKey = federationtypes.ErrInvalidTenantKey
	ErrIPNotAllowed     = federationtypes.ErrIPNotAllowed
	ErrUnauthorized     = federationtypes.ErrUnauthorized

	ErrMissingToken = federationtypes.ErrMissingToken
	ErrInvalidToken = federationtypes.ErrInvalidToken
	ErrGrantRevoked = federationtypes.ErrGrantRevoked

	ErrValidationFailed   = federationtypes.ErrValidationFailed
	ErrInvalidTemplate    = federationtypes.ErrInvalidTemplate
	ErrReservedExternalID = federationtypes.ErrReservedExternalID

	// ErrNotMember — ОДИН сентинел на оба кода-синонима. Спрашивать про факт,
	// а не про имя: ветвление по одному из кодов зелено ровно до той двери,
	// которая отвечает вторым.
	ErrNotMember = federationtypes.ErrNotMember

	ErrNotFound          = federationtypes.ErrNotFound
	ErrOfferNotFound     = federationtypes.ErrOfferNotFound
	ErrProgramNotFound   = federationtypes.ErrProgramNotFound
	ErrNoEligibleProgram = federationtypes.ErrNoEligibleProgram

	ErrProgramInactive     = federationtypes.ErrProgramInactive
	ErrCodeIssuedByOwner   = federationtypes.ErrCodeIssuedByOwner
	ErrCodeNotAvailableYet = federationtypes.ErrCodeNotAvailableYet

	ErrConflict = federationtypes.ErrConflict

	ErrInvalidSignedRef = federationtypes.ErrInvalidSignedRef
	ErrTenantMismatch   = federationtypes.ErrTenantMismatch
	ErrBadgeNotActive   = federationtypes.ErrBadgeNotActive

	ErrInvalidCommissionConfig = federationtypes.ErrInvalidCommissionConfig
	ErrRateUnknown             = federationtypes.ErrRateUnknown
	ErrCascadeNotAvailable     = federationtypes.ErrCascadeNotAvailable
	ErrCascadeFailed           = federationtypes.ErrCascadeFailed
	ErrConversionNotFound      = federationtypes.ErrConversionNotFound
	ErrNotReversible           = federationtypes.ErrNotReversible
	ErrAmountExceedsOriginal   = federationtypes.ErrAmountExceedsOriginal
	ErrReversalFailed          = federationtypes.ErrReversalFailed

	ErrEmailBelongsToDifferentUser = federationtypes.ErrEmailBelongsToDifferentUser
	ErrPhoneBelongsToDifferentUser = federationtypes.ErrPhoneBelongsToDifferentUser
)

// IsNotMember отвечает, означает ли отказ «человек не участник этой сети» —
// каким бы из двух живых кодов дверь его ни назвала.
func IsNotMember(err error) bool { return federationtypes.IsNotMember(err) }

// Codes перечисляет коды, известные клиенту федерации, в устойчивом порядке.
func Codes() []ErrorCode { return federationtypes.Codes() }

// HTTPStatusFor называет статус, с которым код приезжает по контракту. Второе
// значение — знаком ли код клиенту вообще.
func HTTPStatusFor(code ErrorCode) (int, bool) { return federationtypes.HTTPStatusFor(code) }

// SentinelFor возвращает сентинел кода или nil, если код клиенту неизвестен.
func SentinelFor(code ErrorCode) error { return federationtypes.SentinelFor(code) }

// Retryable отвечает, стоит ли повторять запрос, получивший этот отказ.
func Retryable(code ErrorCode, status int) bool { return federationtypes.Retryable(code, status) }
