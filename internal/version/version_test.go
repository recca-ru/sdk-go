package version_test

import (
	"strings"
	"testing"

	"go.recca.ru/sdk/cpa"
	"go.recca.ru/sdk/federation"
	"go.recca.ru/sdk/internal/version"
)

// TestBothClientsReportTheSameVersion — модуль ОДИН, значит и версия одна.
//
// ⚠⚠ Проверка заведена по факту, а не на всякий случай: в выпущенном v0.2.0
// `cpa.Version` рапортовал «0.1.0-dev», а `federation.Version` — «0.2.0-dev».
// Две константы об одном факте разошлись, как расходится любая вторая копия, и
// заметить это можно было только запустив клиент и прочитав User-Agent.
func TestBothClientsReportTheSameVersion(t *testing.T) {
	if cpa.Version != version.SDK {
		t.Errorf("cpa.Version = %q, а модуль — %q", cpa.Version, version.SDK)
	}
	if federation.Version != version.SDK {
		t.Errorf("federation.Version = %q, а модуль — %q", federation.Version, version.SDK)
	}
}

// TestReleasedVersionIsNotMarkedDev — выпуск не называет себя сборкой.
//
// ⚠⚠ Приставка «-dev» в User-Agent делает журнал Recca бесполезным ровно для
// того вопроса, ради которого поле заведено: отличить версию у интегратора от
// чьей-то локальной сборки. В v0.1.0 и v0.2.0 она уехала в выпуск оба раза —
// значит полагаться на то, что её снимут руками перед тегом, нельзя.
//
// ⚠ Тест НЕ запрещает «-dev» в ветке между выпусками: он запрещает его в
// дереве, у которого версия уже объявлена числом. Снять приставку — часть
// выпуска, и теперь у этой части есть исполнитель.
func TestReleasedVersionIsNotMarkedDev(t *testing.T) {
	if strings.Contains(version.SDK, "-dev") {
		t.Errorf("версия модуля %q несёт «-dev» — выпущенный клиент "+
			"представится сборкой разработчика, и журнал Recca не отличит "+
			"его от чьей-то локальной копии", version.SDK)
	}
	if version.SDK == "" {
		t.Error("версия пуста — представляться нечем")
	}
}

// TestUserAgentsCarryTheirSurface — по имени видно, какая дверь ходит.
func TestUserAgentsCarryTheirSurface(t *testing.T) {
	c, err := cpa.New("rtk_test", &cpa.ClientOptions{Environment: cpa.Sandbox})
	if err != nil {
		t.Fatalf("клиент CPA: %v", err)
	}
	f, err := federation.New("rtk_test", &federation.ClientOptions{Environment: federation.Sandbox})
	if err != nil {
		t.Fatalf("клиент федерации: %v", err)
	}

	if got, want := c.UserAgent(), "recca-cpa-go/"+version.SDK; got != want {
		t.Errorf("User-Agent CPA = %q, ждали %q", got, want)
	}
	if got, want := f.UserAgent(), "recca-federation-go/"+version.SDK; got != want {
		t.Errorf("User-Agent федерации = %q, ждали %q", got, want)
	}
	// ⚠ И они РАЗНЫЕ: одинаковый User-Agent у двух клиентов означал бы, что в
	// журналах Recca нельзя отличить, какая дверь пришла.
	if c.UserAgent() == f.UserAgent() {
		t.Error("клиенты представляются одинаково — в журнале их не различить")
	}
}
