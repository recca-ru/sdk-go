package federationtest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.recca.ru/sdk/federation"
)

// TB — то, что Conformance требует от теста. Интерфейс, а не *testing.T, по
// одной причине: иначе «а краснеет ли этот набор вообще» нельзя было бы
// проверить тестом, и он остался бы утверждением о себе самом.
//
// *testing.T этому интерфейсу удовлетворяет.
type TB interface {
	Helper()
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
	Logf(format string, args ...any)
}

// ConformanceOptions — что подкрутить под конкретную площадку.
type ConformanceOptions struct {
	// UserID — участник, на котором идут чтения. Пусто — набор заводит своего
	// через users.Upsert.
	UserID string
	// OfferID — оффер для чеканки ссылки. Пусто — первый из каталога.
	OfferID string
	// ExternalIDPrefix — префикс номеров, которые набор создаёт.
	//
	// ⚠ Пространство номеров общее для СЕТИ: без своего префикса прогон
	// столкнулся бы с живыми заявками интегратора.
	ExternalIDPrefix string
	// SkipWrites — только чтения. Нужен там, где ключу не выданы права записи.
	SkipWrites bool
	// Now — «сейчас» для номеров; нулевое означает time.Now.
	Now time.Time
}

func (o ConformanceOptions) prefix() string {
	if o.ExternalIDPrefix != "" {
		return o.ExternalIDPrefix
	}
	return "conformance"
}

func (o ConformanceOptions) now() time.Time {
	if !o.Now.IsZero() {
		return o.Now
	}
	return time.Now()
}

// ErrNotSandbox — набор отказался идти: дверь назвалась не песочницей.
var ErrNotSandbox = errors.New("federationtest: дверь не назвалась песочницей")

// AssertSandbox спрашивает у ДВЕРИ, песочница ли она, и возвращает отказ, если
// нет.
//
// ⚠⚠ Спрашивается сервер, а не своя конфигурация, и это главное решение всего
// набора. `ClientOptions.Environment` и `BaseURL` берутся из переменных
// окружения, а переменные путают — подменённый адрес отправил бы приёмочный
// прогон создавать конверсии и заявки на ЖИВЫХ деньгах живых партнёров, и
// выглядело бы это как успешная приёмка.
//
// ⚠ Пустое значение — тоже отказ. Оно означает «дверь старше WV-0804 и о среде
// не говорит», то есть НЕЗНАНИЕ, а не «бой». Пропустить его значило бы
// превратить незнание в разрешение — ровно то, что запрещает правило «„не
// знаю“ не превращается в „нет“».
func AssertSandbox(ctx context.Context, c *federation.Client) (*federation.SelfInfo, error) {
	self, _, err := c.Self.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("federationtest: не удалось спросить дверь о среде: %w", err)
	}
	if !self.IsSandbox() {
		named := string(self.Environment)
		if named == "" {
			named = "(не назвала — дверь старше WV-0804)"
		}
		return self, fmt.Errorf(
			"%w: среда %s, адрес %s. Набор создаёт заявки и конверсии, и на бою это живые деньги",
			ErrNotSandbox, named, c.BaseURL())
	}
	return self, nil
}

// Conformance — приёмочный набор против ЖИВОЙ песочницы.
//
// ⚠⚠ Он не заменяет стенд и не заменяется им. Стенд доказывает, что ваш код
// собирает верные запросы и разбирает верные ответы; набор — что так же
// считает платформа. Зелёный стенд при красном наборе означает, что контракт
// разошёлся с реальностью, и узнать об этом лучше здесь, чем на первой живой
// конверсии.
//
// Первым делом набор спрашивает дверь о среде и ОТКАЗЫВАЕТСЯ идти дальше, если
// она не песочница.
func Conformance(ctx context.Context, t TB, c *federation.Client, opts ConformanceOptions) {
	t.Helper()

	self, err := AssertSandbox(ctx, c)
	if err != nil {
		// Fatalf, а не Errorf: продолжать значит начать создавать данные там,
		// где мы только что решили, что создавать их нельзя.
		t.Fatalf("%v", err)
		return
	}
	t.Logf("песочница подтверждена дверью: сеть %s, ключ %s, прав %d",
		self.Organization.Slug, self.Key.Prefix, len(self.Key.Scopes))

	userID := opts.UserID
	if userID == "" {
		if opts.SkipWrites {
			t.Fatalf("federationtest: без UserID набор заводит участника сам, " +
				"а SkipWrites это запрещает — назовите UserID")
			return
		}
		userID = conformanceUpsert(ctx, t, c, opts)
		if userID == "" {
			return
		}
	}

	offerID := conformanceOffers(ctx, t, c, opts)
	conformanceUserReads(ctx, t, c, userID)
	conformanceBalance(ctx, t, c, userID)

	if opts.SkipWrites {
		t.Logf("записи пропущены (SkipWrites): чеканка ссылки, заявка и конверсия НЕ измерены")
		return
	}
	if offerID == "" {
		t.Logf("в каталоге нет офферов: чеканка ссылки и заявка НЕ измерены")
		return
	}
	conformanceLinkAndLead(ctx, t, c, userID, offerID, opts)
}

// conformanceUpsert — сведение личности, оно же проверка идемпотентности по
// контактам.
func conformanceUpsert(ctx context.Context, t TB, c *federation.Client, opts ConformanceOptions) string {
	t.Helper()
	email := fmt.Sprintf("%s-%d@conformance.recca.test", opts.prefix(), opts.now().Unix())

	in := federation.UpsertUserInput{
		Email:         email,
		FirstName:     "Приёмка",
		LastName:      "Конформанс",
		EmailVerified: true,
	}
	first, _, err := c.Users.Upsert(ctx, in)
	if err != nil {
		t.Errorf("users.Upsert: %v", err)
		return ""
	}
	if first.TenantReccaID == "" {
		t.Errorf("users.Upsert не вернул tenant_recca_id — звать /users/:id/* нечем")
		return ""
	}

	// ⚠ Повтор ТЕМИ ЖЕ контактами обязан найти того же человека. Заведи он
	// второго — у сети появился бы дубль, а дерево приведших разошлось бы.
	second, _, err := c.Users.Upsert(ctx, in)
	if err != nil {
		t.Errorf("users.Upsert (повтор): %v", err)
		return first.TenantReccaID
	}
	if second.TenantReccaID != first.TenantReccaID {
		t.Errorf("повтор сведения личности завёл ВТОРОГО человека: %q против %q",
			second.TenantReccaID, first.TenantReccaID)
	}
	if second.IsNew {
		t.Errorf("повтор помечен is_new — сеть сочтёт дубль новым участником")
	}
	return first.TenantReccaID
}

func conformanceOffers(ctx context.Context, t TB, c *federation.Client, opts ConformanceOptions) string {
	t.Helper()
	offers, resp, err := c.Offers.List(ctx, federation.ListOffersParams{
		Page: federation.Page{Take: 20},
	})
	if err != nil {
		t.Errorf("offers.List: %v", err)
		return ""
	}
	for _, o := range offers {
		// ⚠ Каталог федерации обязан быть ТОЛЬКО публичным: черновик, уехавший
		// наружу, — утечка, а не неудобство.
		if o.Visibility != "" && o.Visibility != "public" {
			t.Errorf("в каталоге оффер %s с видимостью %q — наружу уходит непубличное",
				o.ID, o.Visibility)
		}
	}
	if resp.Meta.Total != nil {
		t.Logf("каталог: всего %d, на странице %d", *resp.Meta.Total, len(offers))
	}

	if opts.OfferID != "" {
		return opts.OfferID
	}
	if len(offers) == 0 {
		return ""
	}
	return offers[0].ID
}

func conformanceUserReads(ctx context.Context, t TB, c *federation.Client, userID string) {
	t.Helper()

	if _, _, err := c.Users.Get(ctx, userID); err != nil {
		// ⚠ Дверь отвечает 404 и на «нет», и на «не ваш» — это щит от
		// перебора. Значит отказ здесь диагностируется хуже обычного, и
		// говорить об этом надо вслух.
		t.Errorf("users.Get(%s): %v (⚠ 404 на этой двери означает «нет ИЛИ не ваш»)", userID, err)
	}

	if _, _, err := c.Users.ListLeads(ctx, userID, federation.ListLeadsParams{
		Page: federation.Page{Take: 5},
	}); err != nil {
		t.Errorf("users.ListLeads(%s): %v", userID, err)
	}

	tree, _, err := c.Users.Tree(ctx, userID, federation.TreeParams{Depth: 3})
	if err != nil {
		t.Errorf("users.Tree(%s): %v", userID, err)
		return
	}
	// ⚠⚠ Инвариант приватности: первое колено — суррогат, глубже — необратимый
	// хеш. Нарушь его платформа — и сеть смогла бы перебирать людей Recca.
	for _, level := range tree.Levels {
		for _, node := range level.Nodes {
			switch {
			case level.Depth == 1 && node.Identity.Kind() != federation.IdentityDirect:
				t.Errorf("колено 1: узел вида %q, ждали суррогат", node.Identity.Kind())
			case level.Depth > 1 && node.Identity.Kind() != federation.IdentityOpaque:
				t.Errorf("колено %d: узел вида %q — глубже первого дверь обязана "+
					"отдавать необратимый хеш, иначе сеть перебирает людей Recca",
					level.Depth, node.Identity.Kind())
			}
		}
	}
	if tree.DepthUsed > 5 {
		t.Errorf("дерево ушло на глубину %d — платформенный потолок 5", tree.DepthUsed)
	}
}

func conformanceBalance(ctx context.Context, t TB, c *federation.Client, userID string) {
	t.Helper()
	b, _, err := c.Promos.Balance(ctx, userID)
	if err != nil {
		t.Errorf("promos.Balance(%s): %v", userID, err)
		return
	}

	for _, cur := range b.CurrencyBreakdown {
		// Леджер обязан сходиться сам с собой.
		if got := cur.LifetimeAccrued - cur.LifetimeReversed; got != cur.LifetimeNet {
			t.Errorf("%s: accrued %v − reversed %v = %v, а дверь называет available %v",
				cur.Currency, cur.LifetimeAccrued, cur.LifetimeReversed, got, cur.LifetimeNet)
		}
		// ⚠⚠ А вот кошелёк леджеру равняться НЕ обязан, и это главное, что
		// набор обязан не проверять: после первого вывода они расходятся
		// навсегда. Проверка «равны» была бы зелёной ровно до первой живой
		// выплаты и красной потом — то есть ловила бы норму.
		if cur.WalletCleared != nil && *cur.WalletCleared < 0 {
			t.Errorf("%s: кошелёк отрицательный (%v)", cur.Currency, *cur.WalletCleared)
		}
	}

	if spendable, known := b.Spendable(); known {
		t.Logf("доступно сейчас: %v ₽ (начислено минус отменено: %v ₽)",
			spendable, b.TotalLifetimeNetRUB)
	} else {
		// Не отказ: «не знаю» — законный ответ двери, когда счёт несёт
		// операции ещё какой-то сети.
		t.Logf("⚠ дверь не назвала кошелёк — счёт участника несёт операции другой сети")
	}
}

func conformanceLinkAndLead(
	ctx context.Context, t TB, c *federation.Client,
	userID, offerID string, opts ConformanceOptions,
) {
	t.Helper()

	in := federation.CreateReferralLinkInput{OfferID: offerID}
	first, _, err := c.ReferralLinks.Create(ctx, userID, in)
	if err != nil {
		t.Errorf("referralLinks.Create: %v", err)
		return
	}
	if first.SignedRef == "" {
		t.Errorf("ссылка без signed_ref — приводить по ней некого")
		return
	}

	// ⚠ Повтор на ту же пару обязан вернуть ТУ ЖЕ ссылку: вторая означала бы,
	// что у одного человека на один оффер две метки, и статистика разошлась бы.
	second, _, err := c.ReferralLinks.Create(ctx, userID, in)
	if err != nil {
		t.Errorf("referralLinks.Create (повтор): %v", err)
	} else {
		if second.SignedRef != first.SignedRef {
			t.Errorf("повтор выдал ДРУГУЮ метку: %q против %q", second.SignedRef, first.SignedRef)
		}
		if !second.Reused {
			t.Errorf("повтор не помечен переиспользованием (дверь ответила 201, а не 200)")
		}
	}

	// ⚠⚠ Идемпотентность заявки — по external_id В ТЕЛЕ. Номер берётся один на
	// обе попытки: сгенерируй его заново, и проверка измеряла бы не то.
	externalID := fmt.Sprintf("%s-lead-%d", opts.prefix(), opts.now().UnixNano())
	lead := federation.CreateLeadInput{
		ReccaRef:   first.SignedRef,
		ExternalID: externalID,
		LeadSource: "conformance",
		Contact:    &federation.LeadContact{Name: "Приёмочная заявка"},
	}

	created, _, err := c.Leads.Create(ctx, lead)
	if err != nil {
		t.Errorf("leads.Create: %v", err)
		return
	}
	if created.Idempotent {
		t.Errorf("первая заявка помечена повтором — номер %s уже занят в сети", externalID)
	}

	repeat, _, err := c.Leads.Create(ctx, lead)
	if err != nil {
		t.Errorf("leads.Create (повтор): %v", err)
		return
	}
	if !repeat.Idempotent {
		t.Errorf("ПОВТОР ЗАЯВКИ НЕ РАСПОЗНАН: дверь завела вторую по тому же external_id %s — "+
			"это двойная оплата приведшему", externalID)
	}
	if repeat.LeadID != created.LeadID {
		t.Errorf("повтор вернул другую заявку: %q против %q", repeat.LeadID, created.LeadID)
	}
}

// DescribeScopes — человекочитаемый перечень прав ключа: что набор сможет
// измерить, а что нет.
//
// ⚠ Нужен потому, что непройденная часть набора выглядит так же, как
// пройденная: молчание. Печатать, чего мы НЕ измерили, — часть честного
// отчёта.
func DescribeScopes(self *federation.SelfInfo) string {
	if self == nil || len(self.Key.Scopes) == 0 {
		return "прав у ключа не названо"
	}
	return strings.Join(self.Key.Scopes, ", ")
}
