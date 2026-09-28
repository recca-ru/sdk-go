package federation

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"go.recca.ru/sdk/federationtypes"
	"go.recca.ru/sdk/internal/transport"
	"go.recca.ru/sdk/internal/version"
)

// Version — версия SDK. Уходит в User-Agent: по нему в журналах Recca видно,
// какая версия клиента ходит, и это единственный способ узнать это без вопроса
// интегратору.
const Version = version.SDK

// userAgentPrefix — имя клиента в User-Agent.
const userAgentPrefix = "recca-federation-go/"

// Environment — дверь, в которую ходит клиент.
type Environment string

const (
	// Production — боевая дверь: живые люди и живые деньги.
	Production Environment = "production"
	// Sandbox — песочница приёмки: отдельная база, письма никому не уходят.
	Sandbox Environment = "sandbox"
)

// environmentBaseURL — адреса сред. ⚠ Неизвестная среда — ошибка, а не
// молчаливый прод: опечатка в конфигурации не должна отправить приёмочный
// прогон на боевые деньги.
var environmentBaseURL = map[Environment]string{
	Production: "https://api.recca.ru",
	Sandbox:    "https://api-sandbox.recca.ru",
}

// ClientOptions — настройки клиента. Читаются ОДИН раз, при New: правка этой
// структуры после создания на клиента не действует.
type ClientOptions struct {
	// Environment — среда; пустое значение означает Production.
	Environment Environment
	// BaseURL — явный адрес двери. Сильнее Environment; нужен стендам и
	// локальным прогонам.
	BaseURL string
	// HTTPClient — свой клиент: таймауты, прокси, пул соединений. Без него
	// берётся клиент с таймаутом в 30 секунд.
	HTTPClient *http.Client
	// UserAgent — своё имя вместо recca-federation-go/<версия>.
	UserAgent string
	// MaxRetries — сколько раз повторять идемпотентную операцию. nil означает
	// умолчание, Ptr(0) — «повторов нет».
	//
	// ⚠ Указатель, а не число: у int ноль означал бы одновременно «не задали»
	// и «повторять запрещено», а это разные распоряжения.
	MaxRetries *int
	// Logger — отладочный вывод: метод, путь, статус, попытка, request_id.
	// Тела запроса и ответа не пишутся никогда.
	Logger *slog.Logger
	// LogHeaders добавляет к отладке заголовки по списку; ключ арендатора в
	// них маскируется.
	LogHeaders bool
}

// defaultMaxRetries — повторов по умолчанию. Две попытки сверх первой: этого
// хватает, чтобы пережить перезапуск бэкенда, и мало, чтобы усугубить его
// падение.
const defaultMaxRetries = 2

// Client — клиент федеративного API Recca.
//
// ⚠ Клиент неизменяем: ключ, среда и транспорт задаются один раз. Сеттеров
// нет намеренно — горутина, меняющая ключ на лету, породила бы гонку, которой
// не видно ни в одном тесте потребителя.
type Client struct {
	exec      *transport.Executor
	baseURL   string
	userAgent string

	// common — общая часть сервисов, ОДНА на клиента: каждый сервис есть тот же
	// самый service под своим именем, а поля ниже — указатели на эту структуру,
	// а не её копии. Устройство и довод — у типа service.
	common service

	// Self — сведения о сети, о ключе и о СРЕДЕ, в которой стоит дверь.
	Self *SelfService
	// Users — сведение личности, карточка, заявки человека, его дерево.
	Users *UsersService
	// ReferralLinks — чеканка реферальных ссылок от имени участника.
	ReferralLinks *ReferralLinksService
	// Offers — каталог офферов сети.
	Offers *OffersService
	// Leads — приём заявок.
	Leads *LeadsService
	// Conversions — приём результата и его отмена.
	Conversions *ConversionsService
	// Promos — баланс участника по программам сети.
	Promos *PromosService
	// Badges — бейджи ролей.
	Badges *BadgesService
}

// New собирает клиент. Ключ арендатора обязателен: его отсутствие — ошибка
// конфигурации, и узнать о ней лучше здесь, чем по чужому коду 401.
//
//	c, err := federation.New(os.Getenv("RECCA_TENANT_KEY"),
//	    &federation.ClientOptions{Environment: federation.Sandbox})
func New(key string, opts *ClientOptions) (*Client, error) {
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("federation: ключ арендатора не задан: %w", federationtypes.ErrValidation)
	}

	// Копия: дальше клиент живёт своей жизнью, а структура вызывающего — своей.
	var o ClientOptions
	if opts != nil {
		o = *opts
	}

	baseURL, err := resolveBaseURL(o)
	if err != nil {
		return nil, err
	}

	userAgent := o.UserAgent
	if userAgent == "" {
		userAgent = userAgentPrefix + Version
	}

	maxRetries := defaultMaxRetries
	if o.MaxRetries != nil {
		maxRetries = *o.MaxRetries
		if maxRetries < 0 {
			return nil, fmt.Errorf("federation: число повторов %d отрицательно: %w",
				maxRetries, federationtypes.ErrValidation)
		}
	}

	exec, err := transport.New(transport.Options{
		Surface:    "federation",
		BaseURL:    baseURL,
		Key:        key,
		UserAgent:  userAgent,
		HTTPClient: o.HTTPClient,
		MaxRetries: maxRetries,
		Logger:     o.Logger,
		LogHeaders: o.LogHeaders,
	})
	if err != nil {
		return nil, err
	}

	c := &Client{exec: exec, baseURL: baseURL, userAgent: userAgent}
	c.common.client = c
	c.Self = (*SelfService)(&c.common)
	c.Users = (*UsersService)(&c.common)
	c.ReferralLinks = (*ReferralLinksService)(&c.common)
	c.Offers = (*OffersService)(&c.common)
	c.Leads = (*LeadsService)(&c.common)
	c.Conversions = (*ConversionsService)(&c.common)
	c.Promos = (*PromosService)(&c.common)
	c.Badges = (*BadgesService)(&c.common)
	return c, nil
}

func resolveBaseURL(o ClientOptions) (string, error) {
	if o.BaseURL != "" {
		return strings.TrimRight(o.BaseURL, "/"), nil
	}
	env := o.Environment
	if env == "" {
		env = Production
	}
	base, ok := environmentBaseURL[env]
	if !ok {
		return "", fmt.Errorf("federation: неизвестная среда %q, допустимы %q и %q: %w",
			env, Production, Sandbox, federationtypes.ErrValidation)
	}
	return base, nil
}

// BaseURL — адрес двери, в которую ходит клиент.
func (c *Client) BaseURL() string { return c.baseURL }

// UserAgent — имя, которым клиент представляется.
func (c *Client) UserAgent() string { return c.userAgent }

// Do отправляет произвольный запрос к двери: path задаётся ОТ версии
// (`/users/upsert`), версионный префикс `/federation/v1` подставляется сам.
// Параметры строки запроса пишутся прямо в path (`/offers?take=20`).
//
// Метод существует для того, чего ещё нет в типизированных сервисах: контракт
// растёт, а ждать волны SDK ради нового поля интегратору незачем.
//
// ⚠⚠ Повторяются только запросы, читающие данные (GET, HEAD). POST не
// повторяется здесь никогда: ответ мог потеряться уже после того, как
// конверсия записана, и повтор оплатил бы её вторично. Идемпотентные POST
// повторяют типизированные методы — они знают, чем у операции различается
// повтор.
func (c *Client) Do(ctx context.Context, method, path string, body, out any) (*Response, error) {
	return c.exec.Do(ctx, request{
		Method: method,
		Path:   path,
		Body:   body,
	}.transport(out))
}

func isReadMethod(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead:
		return true
	}
	return false
}
