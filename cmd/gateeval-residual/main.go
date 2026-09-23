// Command gateeval-residual builds the REAL residual dataset for the gate
// benchmark. Each gateeval sample is treated as an ORIGINAL chunk:
//
//	original -> Rule Engine -> residual (mask strong entities) -> Heuristic Gate
//
// The residual is what the ML gates (LLAIM / spaCy) actually see in production.
// Ground truth after rules:
//
//	positive = at least one gold PII value remains in the residual
//	negative = Rule Engine masked all gold PII, or the original had no PII
//
// Gold PII values are defined per sample (the PII the sample is designed to
// contain). Routing (SAFE / UNCERTAIN / LIKELY_PII) is produced by the real
// heuristic gate on the residual.
//
// Output JSON:
//
//	{
//	  "samples": [
//	    {"original": "...", "residual": "...", "has_pii": 0|1,
//	     "route": "SAFE|UNCERTAIN|LIKELY_PII", "gold_pii": ["..."]}
//	  ]
//	}
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kryneuse/alpha_proxy/internal/engine"
	"github.com/kryneuse/alpha_proxy/internal/gate"
	"github.com/kryneuse/alpha_proxy/internal/gateeval"
	"github.com/kryneuse/alpha_proxy/internal/residual"
)

type sample struct {
	Original string   `json:"original"`
	Residual string   `json:"residual"`
	HasPII   int      `json:"has_pii"`
	Route    string   `json:"route"`
	GoldPII  []string `json:"gold_pii"`
}

type dataset struct {
	Samples []sample `json:"samples"`
}

// goldPII maps each HasPII=1 gateeval sample to the PII value(s) it is
// designed to contain. Used to determine whether PII survives rule masking.
var goldPII = map[string][]string{
	"4510 123456":                          {"4510 123456"},
	"паспорт 4510 123456":                  {"4510 123456"},
	"серия 4510 номер 123456":              {"4510 123456"},
	"7712 345678":                          {"7712 345678"},
	"водительское удостоверение 77 12 345678": {"77 12 345678"},
	"7707083893":                           {"7707083893"},
	"ИНН 7707083893":                       {"7707083893"},
	"4532 0151 1283 0366":                  {"4532 0151 1283 0366"},
	"номер карты 4532 0151 1283 0366":      {"4532 0151 1283 0366"},
	"+7 (912) 345-67-89":                   {"+7 (912) 345-67-89"},
	"телефон +7 (912) 345-67-89":           {"+7 (912) 345-67-89"},
	"ivanov@example.com":                   {"ivanov@example.com"},
	"email ivanov@example.com":             {"ivanov@example.com"},
	"15.03.1990":                           {"15.03.1990"},
	"дата рождения 15.03.1990":             {"15.03.1990"},
	"770-001":                              {"770-001"},
	"код подразделения 770-001":            {"770-001"},
	"Иван Петров":                          {"Иван Петров"},
	"клиент Иван Петров":                   {"Иван Петров"},
	"Смирнова Анна Сергеевна":              {"Смирнова Анна Сергеевна"},
	"заявитель Смирнова Анна":              {"Смирнова Анна"},
	"проживает по адресу г. Москва ул. Тверская д. 10": {"г. Москва ул. Тверская д. 10"},
	"адрес регистрации г. Казань ул. Баумана д. 7":     {"г. Казань ул. Баумана д. 7"},
	"гражданство Российская Федерация":     {"Российская Федерация"},
	"гражданин Казахстана":                 {"Казахстана"},
	"родился в городе Санкт-Петербург":     {"Санкт-Петербург"},
	"место рождения город Москва":          {"Москва"},
	"CVV 123":                              {"123"},
	"пин-код 7305":                         {"7305"},
	"держатель карты IVAN PETROV":          {"IVAN PETROV"},
	"паспорт выдан ОВМ УМВД России":        {"ОВМ УМВД России"},
	"код подразделения 452-013":            {"452-013"},
	"водительские права 9901 234567":       {"9901 234567"},
	"ИНН физического лица 500100732259":    {"500100732259"},
	"номер счёта 40817810099910004312":     {"40817810099910004312"},
	"паспорт 45 10 № 123456":               {"45 10 № 123456"},
	"серия 45 10, номер 123456":            {"45 10, номер 123456"},
	"является гражданином Армении":         {"Армении"},
	"Гражданка Республики Беларусь":        {"Республики Беларусь"},
	"родился в 1990 году":                  {"1990"},
	"дата рождения 12 января 1985":         {"12 января 1985"},
	"живет по адресу г. Новосибирск":       {"г. Новосибирск"},
	"зарегистрирован по адресу г. Екатеринбург": {"г. Екатеринбург"},
	"клиент Кузнецова Мария":               {"Кузнецова Мария"},
	"пациент Волков Дмитрий":               {"Волков Дмитрий"},
	"заёмщик Сидоров Алексей":              {"Сидоров Алексей"},
	"держатель полиса Петрова Анна":        {"Петрова Анна"},
	"Абдуллаев Тимур Рашидович":            {"Абдуллаев Тимур Рашидович"},
	"клиент Абдуллаев Тимур":               {"Абдуллаев Тимур"},
	"проживает в деревне Малые Ключи":      {"Малые Ключи"},
	"адрес: посёлок Сосновый Бор, улица Лесная": {"Сосновый Бор, улица Лесная"},
	"родился в селе Верхние Поляны":        {"Верхние Поляны"},
	"1234 567890":                          {"1234 567890"},
	"9876 543210":                          {"9876 543210"},
	"123456789012":                         {"123456789012"},
	"1234567890":                           {"1234567890"},
	"4111 1111 1111 1111":                  {"4111 1111 1111 1111"},
	"5555 5555 5555 4444":                  {"5555 5555 5555 4444"},
	"код 123":                              {"123"},
	"номер 4567":                           {"4567"},
	"пароль 1234":                          {"1234"},
	"код доступа 7305":                     {"7305"},
}

func main() {
	out := os.Args[1]
	if out == "" {
		fmt.Fprintln(os.Stderr, "usage: gateeval-residual <output.json>")
		os.Exit(2)
	}

	e := engine.New(engine.Options{})
	g := gate.New(gate.DefaultConfig())
	src := gateeval.Dataset()

	ds := dataset{Samples: make([]sample, 0, len(src))}
	for _, s := range src {
		ents := e.Analyze(s.Text)
		res := residual.Build(s.Text, ents)
		trimmed := strings.TrimSpace(res)

		// Ground truth after rules: does any gold PII survive masking?
		hasPII := 0
		if s.HasPII == 1 {
			for _, gold := range goldPII[s.Text] {
				if strings.Contains(trimmed, gold) {
					hasPII = 1
					break
				}
			}
		}

		route := g.Evaluate(trimmed).Route

		ds.Samples = append(ds.Samples, sample{
			Original: s.Text,
			Residual: trimmed,
			HasPII:   hasPII,
			Route:    string(route),
			GoldPII:  goldPII[s.Text],
		})
	}

	b, err := json.MarshalIndent(ds, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "marshal:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(out, b, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %d samples to %s\n", len(ds.Samples), out)
}