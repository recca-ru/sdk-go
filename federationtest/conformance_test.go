package federationtest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func ctx() context.Context { return context.Background() }

// recorder — TB, который запоминает жалобы вместо того, чтобы ронять тест.
//
// ⚠⚠ Он и есть причина, по которой Conformance принимает интерфейс, а не
// *testing.T. Без него «набор краснеет на сломанном» осталось бы утверждением
// о себе самом: зелёный прогон выглядит одинаково и когда набор проверил, и
// когда он проверять разучился.
type recorder struct {
	errs  []string
	fatal []string
	logs  []string
}

func (r *recorder) Helper() {}
func (r *recorder) Errorf(format string, args ...any) {
	r.errs = append(r.errs, sprintf(format, args...))
}
func (r *recorder) Fatalf(format string, args ...any) {
	r.fatal = append(r.fatal, sprintf(format, args...))
}
func (r *recorder) Logf(format string, args ...any) {
	r.logs = append(r.logs, sprintf(format, args...))
}
func (r *recorder) complaints() string {
	return strings.Join(append(append([]string{}, r.errs...), r.fatal...), "\n")
}

func sprintf(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

// ── отказ идти не по песочнице ───────────────────────────────────────────────

// TestConformanceRefusesNonSandbox — главная защита набора.
func TestConformanceRefusesNonSandbox(t *testing.T) {
	for _, env := range []string{"production", "staging"} {
		t.Run(env, func(t *testing.T) {
			s := New(t)
			s.setEnvironment(env)
			c := s.Client(t)

			rec := &recorder{}
			Conformance(ctx(), rec, c, ConformanceOptions{UserID: "trid_conformance_01"})

			if len(rec.fatal) == 0 {
				t.Fatalf("набор пошёл в среду %q — на бою он создаёт заявки и "+
					"конверсии живым партнёрам", env)
			}
			if !strings.Contains(rec.complaints(), env) {
				t.Errorf("отказ не назвал среду: %s", rec.complaints())
			}

			// ⚠⚠ И главное: после отказа набор не сделал НИ ОДНОГО обращения,
			// кроме вопроса о среде. Отказ, успевший что-то создать, защитой
			// не является.
			reqs := s.Requests()
			if len(reqs) != 1 || reqs[0].Path != "/self" {
				paths := make([]string, 0, len(reqs))
				for _, r := range reqs {
					paths = append(paths, r.Method+" "+r.Path)
				}
				t.Errorf("после отказа набор сходил ещё: %v", paths)
			}
		})
	}
}

// TestConformanceRefusesUnnamedEnvironment — «не знаю» не превращается в «да».
//
// ⚠⚠ Пустое поле означает дверь старше WV-0804, то есть НЕЗНАНИЕ. Пропустить
// его значило бы пойти создавать данные там, где неизвестно, что это за
// площадка, — и именно так выглядела бы приёмка против старого прода.
//
// ⚠⚠ Три случая, и их СТАЛО три после мутационного прогона 28.09.2026.
// Прежняя редакция проверяла один — пробел, — и мутация «пропускать среду,
// если поле пустое» осталась ЗЕЛЁНОЙ: пробел пустым не является, и тест
// измерял не тот случай, оставаясь при этом зелёным и выглядя проверкой.
// «Поля нет» и «поле пустое» приезжают по сети по-разному, а Go оба разбирает
// в "" — значит послать надо оба.
func TestConformanceRefusesUnnamedEnvironment(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*Server)
	}{
		{"поля нет вовсе (дверь старше WV-0804)", func(s *Server) { s.dropEnvironmentField() }},
		{"поле пустое", func(s *Server) { s.setEnvironment("") }},
		{"поле из пробела", func(s *Server) { s.setEnvironment(" ") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := New(t)
			tc.setup(s)
			c := s.Client(t)

			rec := &recorder{}
			Conformance(ctx(), rec, c, ConformanceOptions{UserID: "trid_conformance_01"})

			if len(rec.fatal) == 0 {
				t.Fatal("набор пошёл в среду, которую дверь не назвала")
			}
			// И ничего не создал: отказ, успевший записать, защитой не является.
			if reqs := s.Requests(); len(reqs) != 1 || reqs[0].Path != "/self" {
				t.Errorf("после отказа набор сходил ещё: %d обращений", len(reqs))
			}
		})
	}
}

func TestAssertSandboxAcceptsSandbox(t *testing.T) {
	s := New(t)
	c := s.Client(t)

	self, err := AssertSandbox(ctx(), c)
	if err != nil {
		t.Fatalf("песочница отвергнута: %v", err)
	}
	if !self.IsSandbox() {
		t.Error("IsSandbox() ложно на песочнице")
	}
	if errors.Is(err, ErrNotSandbox) {
		t.Error("на песочнице вернулся ErrNotSandbox")
	}
}

// ── набор проходит целиком и краснеет на сломанном ───────────────────────────

func TestConformanceGreenAgainstStand(t *testing.T) {
	s := New(t)
	c := s.Client(t)

	rec := &recorder{}
	Conformance(ctx(), rec, c, ConformanceOptions{
		UserID:           "trid_conformance_01",
		ExternalIDPrefix: "standrun",
	})

	if len(rec.fatal) > 0 || len(rec.errs) > 0 {
		t.Fatalf("набор покраснел на исправном стенде:\n%s", rec.complaints())
	}
	// ⚠ «Зелёный достижим» проверяется отдельно от «краснеет на сломанном»:
	// набор, который зелен всегда, и набор, который красен всегда, одинаково
	// бесполезны, а выглядят по-разному только вместе.
	if len(rec.logs) == 0 {
		t.Error("набор не сказал о себе НИЧЕГО — отчёт без единой строки " +
			"неотличим от непройденного набора")
	}
}

// TestConformanceCatchesBrokenLeadIdempotency — мутация: дверь перестала
// распознавать повтор заявки.
//
// ⚠⚠ Это самый дорогой из проверяемых отказов: повтор, принятый как новая
// заявка, платит приведшему второй раз, и ни один код ответа об этом не
// говорит — обе доставки успешны.
func TestConformanceCatchesBrokenLeadIdempotency(t *testing.T) {
	s := New(t)
	// Стенд начинает отвечать на повтор 201, как на новую строку.
	s.repeatStatus = 201
	s.breakIdempotency = true
	c := s.Client(t)

	rec := &recorder{}
	Conformance(ctx(), rec, c, ConformanceOptions{
		UserID:           "trid_conformance_01",
		ExternalIDPrefix: "broken",
	})

	if !strings.Contains(rec.complaints(), "ПОВТОР ЗАЯВКИ НЕ РАСПОЗНАН") {
		t.Fatalf("набор НЕ заметил сломанную идемпотентность заявки — "+
			"значит зелёный прогон ничего о ней не доказывает.\nЧто он сказал:\n%s",
			rec.complaints())
	}
}

// TestConformanceCatchesTreePrivacyBreach — мутация: дверь отдала на втором
// колене суррогат вместо необратимого хеша.
func TestConformanceCatchesTreePrivacyBreach(t *testing.T) {
	s := New(t)
	s.tamper = func(path string, data json.RawMessage) json.RawMessage {
		if !strings.HasSuffix(path, "/tree") {
			return data
		}
		// Подменяем хеш второго колена на суррогат: ровно то, что позволило бы
		// сети перебирать людей Recca.
		return json.RawMessage(strings.Replace(string(data),
			`"hashed_user_id"`, `"tenant_recca_id"`, 1))
	}
	c := s.Client(t)

	rec := &recorder{}
	Conformance(ctx(), rec, c, ConformanceOptions{
		UserID:           "trid_conformance_01",
		ExternalIDPrefix: "privacy",
	})

	if !strings.Contains(rec.complaints(), "необратимый хеш") {
		t.Fatalf("набор НЕ заметил, что второе колено приехало суррогатом:\n%s",
			rec.complaints())
	}
}
