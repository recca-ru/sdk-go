package federationtypes

// DeploymentEnvironment — среда, которую называет САМ СЕРВЕР.
//
// ⚠⚠ Спрашивать её у двери, а не выводить из адреса клиента, — единственный
// способ узнать правду: адрес берётся из конфигурации, а конфигурацию путают.
// Набор приёмки, поверивший своей настройке, пошёл бы создавать заявки и
// конверсии на БОЮ, будучи уверенным, что он в песочнице.
type DeploymentEnvironment string

const (
	// EnvProduction — бой: живые люди и живые деньги.
	EnvProduction DeploymentEnvironment = "production"
	// EnvStaging — предпрод.
	EnvStaging DeploymentEnvironment = "staging"
	// EnvSandbox — песочница приёмки: отдельная база, письма никому не уходят.
	EnvSandbox DeploymentEnvironment = "sandbox"
)

// SelfOrganization — сеть, которой принадлежит ключ.
type SelfOrganization struct {
	ID          string `json:"id" federation:"required"`
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
	PlanTier    string `json:"plan_tier"`
	// AllowedOrigins — источники, которым платформа разрешит обращаться из
	// браузера.
	AllowedOrigins []string `json:"allowed_origins"`
}

// SelfKey — предъявленный ключ.
type SelfKey struct {
	// Prefix — начало ключа, годное для показа в интерфейсе. Сам ключ дверь не
	// отдаёт никогда.
	Prefix string `json:"prefix"`
	// Scopes — права, выданные этому ключу.
	Scopes []string `json:"scopes"`
}

// SelfVisibility — что ключу видно.
type SelfVisibility struct {
	// ShopCount — сколько витрин сети попадает в область видимости.
	ShopCount int `json:"shop_count"`
}

// SelfInfo — ответ GET /federation/v1/self.
type SelfInfo struct {
	Organization SelfOrganization `json:"organization" federation:"required"`
	Key          SelfKey          `json:"key"`
	Visibility   SelfVisibility   `json:"visibility"`

	// Environment — среда развёртывания, названная сервером.
	//
	// ⚠ Пусто означает «дверь старше WV-0804 и о среде не говорит». Это НЕ
	// «production»: подставить сюда бой значило бы выдать незнание за
	// утверждение. Набор приёмки на пустом значении отказывается идти — по той
	// же причине, по которой отказывается на «production».
	Environment DeploymentEnvironment `json:"environment"`
}

// IsSandbox отвечает, назвался ли сервер песочницей.
//
// ⚠ Отдельная функция, а не сравнение у потребителя: «не production» и
// «sandbox» — разные утверждения, и первое истинно при пустом поле, то есть
// при НЕЗНАНИИ.
func (s SelfInfo) IsSandbox() bool { return s.Environment == EnvSandbox }
