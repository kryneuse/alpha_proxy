// Package eval provides an evaluation dataset and metrics computation for the
// rule engine. All data is synthetic.
package eval

import (
	"fmt"
	"strings"

	"github.com/kryneuse/alpha_proxy/internal/entity"
)

// Expected is an expected entity with an exact span.
type Expected struct {
	Type  entity.Type
	Text  string
	Start int
	End   int
}

// Sample is a single evaluation case.
type Sample struct {
	Text     string
	Expected []Expected
	// Negative marks a hard-negative case (no entities expected).
	Negative bool
}

// exp builds an Expected entity by locating text within the sample text.
// It panics if the text is not found, ensuring the dataset is always valid.
func exp(text, typ, value string) Expected {
	idx := strings.Index(text, value)
	if idx < 0 {
		panic(fmt.Sprintf("expected text %q not found in %q", value, text))
	}
	return Expected{Type: entity.Type(typ), Text: value, Start: idx, End: idx + len(value)}
}

// Dataset returns the evaluation dataset.
func Dataset() []Sample {
	return []Sample{
		// ================= FULL_NAME =================
		{Text: "Клиент: Иванов Иван Петрович", Expected: []Expected{exp("Клиент: Иванов Иван Петрович", "FULL_NAME", "Иванов Иван Петрович")}},
		{Text: "ФИО заявителя: Петрова Анна Сергеевна", Expected: []Expected{exp("ФИО заявителя: Петрова Анна Сергеевна", "FULL_NAME", "Петрова Анна Сергеевна")}},
		{Text: "Сотрудник Иван Петров подал заявку", Expected: []Expected{exp("Сотрудник Иван Петров подал заявку", "FULL_NAME", "Иван Петров")}},
		{Text: "Заёмщик: Сидоров Алексей Викторович", Expected: []Expected{exp("Заёмщик: Сидоров Алексей Викторович", "FULL_NAME", "Сидоров Алексей Викторович")}},
		{Text: "Держатель полиса Кузнецова Мария", Expected: []Expected{exp("Держатель полиса Кузнецова Мария", "FULL_NAME", "Кузнецова Мария")}},
		{Text: "Пациент: Волков Дмитрий Сергеевич", Expected: []Expected{exp("Пациент: Волков Дмитрий Сергеевич", "FULL_NAME", "Волков Дмитрий Сергеевич")}},

		// ================= BIRTH_DATE =================
		{Text: "Дата рождения: 15.03.1990", Expected: []Expected{exp("Дата рождения: 15.03.1990", "BIRTH_DATE", "15.03.1990")}},
		{Text: "Родился 01-02-2000", Expected: []Expected{exp("Родился 01-02-2000", "BIRTH_DATE", "01-02-2000")}},
		{Text: "Дата рождения 12 января 1985", Expected: []Expected{exp("Дата рождения 12 января 1985", "BIRTH_DATE", "12 января 1985")}},
		{Text: "Родилась 05/11/1992", Expected: []Expected{exp("Родилась 05/11/1992", "BIRTH_DATE", "05/11/1992")}},
		{Text: "День рождения: 30.06.1978", Expected: []Expected{exp("День рождения: 30.06.1978", "BIRTH_DATE", "30.06.1978")}},
		{Text: "Год рождения 1988", Expected: []Expected{exp("Год рождения 1988", "BIRTH_DATE", "1988")}},
		{Text: "Дата рождения: 1990-03-15", Expected: []Expected{exp("Дата рождения: 1990-03-15", "BIRTH_DATE", "1990-03-15")}},
		{Text: "Родился в 1990 году в Москве", Expected: []Expected{exp("Родился в 1990 году в Москве", "BIRTH_DATE", "1990"), exp("Родился в 1990 году в Москве", "BIRTH_PLACE", "Москве")}},

		// ================= BIRTH_PLACE =================
		{Text: "Место рождения: город Москва", Expected: []Expected{exp("Место рождения: город Москва", "BIRTH_PLACE", "город Москва")}},
		{Text: "Родился в городе Санкт-Петербург", Expected: []Expected{exp("Родился в городе Санкт-Петербург", "BIRTH_PLACE", "городе Санкт-Петербург")}},
		{Text: "Место рождения: Москва, дата рождения 15.03.1990", Expected: []Expected{exp("Место рождения: Москва, дата рождения 15.03.1990", "BIRTH_PLACE", "Москва"), exp("Место рождения: Москва, дата рождения 15.03.1990", "BIRTH_DATE", "15.03.1990")}},
		{Text: "Уроженец города Казань", Expected: []Expected{exp("Уроженец города Казань", "BIRTH_PLACE", "города Казань")}},
		{Text: "Родилась в Новосибирске", Expected: []Expected{exp("Родилась в Новосибирске", "BIRTH_PLACE", "Новосибирске")}},

		// ================= PASSPORT =================
		{Text: "Паспорт 4510 123456", Expected: []Expected{exp("Паспорт 4510 123456", "PASSPORT", "4510 123456")}},
		{Text: "Серия 45 10 номер 123456", Expected: []Expected{exp("Серия 45 10 номер 123456", "PASSPORT", "Серия 45 10 номер 123456")}},
		{Text: "Паспорт гражданина РФ 7712 345678", Expected: []Expected{exp("Паспорт гражданина РФ 7712 345678", "PASSPORT", "7712 345678"), exp("Паспорт гражданина РФ 7712 345678", "CITIZENSHIP", "РФ")}},
		{Text: "Серия и номер паспорта: 4510 123456", Expected: []Expected{exp("Серия и номер паспорта: 4510 123456", "PASSPORT", "4510 123456")}},

		// ================= CITIZENSHIP =================
		{Text: "Гражданство: Российская Федерация", Expected: []Expected{exp("Гражданство: Российская Федерация", "CITIZENSHIP", "Российская Федерация")}},
		{Text: "Гражданин РФ", Expected: []Expected{exp("Гражданин РФ", "CITIZENSHIP", "РФ")}},
		{Text: "Гражданство: РФ.", Expected: []Expected{exp("Гражданство: РФ.", "CITIZENSHIP", "РФ")}},
		{Text: "Гражданка Республики Беларусь", Expected: []Expected{exp("Гражданка Республики Беларусь", "CITIZENSHIP", "Республики Беларусь")}},
		{Text: "Гражданство: Казахстан", Expected: []Expected{exp("Гражданство: Казахстан", "CITIZENSHIP", "Казахстан")}},

		// ================= PASSPORT_ISSUER =================
		{Text: "Паспорт выдан ОВМ УМВД России по г. Москве", Expected: []Expected{exp("Паспорт выдан ОВМ УМВД России по г. Москве", "PASSPORT_ISSUER", "ОВМ УМВД России по г. Москве")}},
		{Text: "Паспорт выдан ОВМ УМВД России по г. Москве 20.04.2010", Expected: []Expected{exp("Паспорт выдан ОВМ УМВД России по г. Москве 20.04.2010", "PASSPORT_ISSUER", "ОВМ УМВД России по г. Москве")}},
		{Text: "Выдан ОМВД России по району Хамовники", Expected: []Expected{exp("Выдан ОМВД России по району Хамовники", "PASSPORT_ISSUER", "ОМВД России по району Хамовники")}},
		{Text: "Кем выдан: УФМС России по г. Санкт-Петербургу", Expected: []Expected{exp("Кем выдан: УФМС России по г. Санкт-Петербургу", "PASSPORT_ISSUER", "УФМС России по г. Санкт-Петербургу")}},

		// ================= DEPARTMENT_CODE =================
		{Text: "Код подразделения 770-001", Expected: []Expected{exp("Код подразделения 770-001", "DEPARTMENT_CODE", "770-001")}},
		{Text: "Паспорт, код подразделения 452-013", Expected: []Expected{exp("Паспорт, код подразделения 452-013", "DEPARTMENT_CODE", "452-013")}},
		{Text: "Код подразделения: 770-123", Expected: []Expected{exp("Код подразделения: 770-123", "DEPARTMENT_CODE", "770-123")}},

		// ================= PASSPORT_ISSUE_DATE =================
		{Text: "Паспорт выдан 20.04.2010", Expected: []Expected{exp("Паспорт выдан 20.04.2010", "PASSPORT_ISSUE_DATE", "20.04.2010")}},
		{Text: "Дата выдачи: 05.11.2015", Expected: []Expected{exp("Дата выдачи: 05.11.2015", "PASSPORT_ISSUE_DATE", "05.11.2015")}},
		{Text: "Дата выдачи паспорта: 20.07.2015", Expected: []Expected{exp("Дата выдачи паспорта: 20.07.2015", "PASSPORT_ISSUE_DATE", "20.07.2015")}},
		{Text: "Родился 15 марта 1990 года", Expected: []Expected{exp("Родился 15 марта 1990 года", "BIRTH_DATE", "15 марта 1990")}},
		{Text: "Паспорт выдан 15 марта 2008", Expected: []Expected{exp("Паспорт выдан 15 марта 2008", "PASSPORT_ISSUE_DATE", "15 марта 2008")}},

		// ================= DRIVER_LICENSE =================
		{Text: "Водительское удостоверение 7712 345678", Expected: []Expected{exp("Водительское удостоверение 7712 345678", "DRIVER_LICENSE", "7712 345678")}},
		{Text: "Водительские права 4510 123456", Expected: []Expected{exp("Водительские права 4510 123456", "DRIVER_LICENSE", "4510 123456")}},
		{Text: "В/у 7712 345678", Expected: []Expected{exp("В/у 7712 345678", "DRIVER_LICENSE", "7712 345678")}},
		{Text: "Водительское удостоверение 77 12 345678", Expected: []Expected{exp("Водительское удостоверение 77 12 345678", "DRIVER_LICENSE", "77 12 345678")}},
		{Text: "ВУ: 77 12 345678", Expected: []Expected{exp("ВУ: 77 12 345678", "DRIVER_LICENSE", "77 12 345678")}},
		{Text: "Водительское удостоверение 77-12 345678", Expected: []Expected{exp("Водительское удостоверение 77-12 345678", "DRIVER_LICENSE", "77-12 345678")}},
		{Text: "Водительское удостоверение 77 12 №345678", Expected: []Expected{exp("Водительское удостоверение 77 12 №345678", "DRIVER_LICENSE", "77 12 №345678")}},

		// ================= ADDRESS =================
		{Text: "Адрес: г. Москва, ул. Тверская, д. 10, кв. 5", Expected: []Expected{exp("Адрес: г. Москва, ул. Тверская, д. 10, кв. 5", "ADDRESS", "г. Москва, ул. Тверская, д. 10, кв. 5")}},
		{Text: "Адрес регистрации: 101000, г. Москва, ул. Арбат, д. 1", Expected: []Expected{exp("Адрес регистрации: 101000, г. Москва, ул. Арбат, д. 1", "ADDRESS", "101000, г. Москва, ул. Арбат, д. 1")}},
		{Text: "Проживает по адресу: г. Казань, ул. Баумана, д. 7, кв. 12", Expected: []Expected{exp("Проживает по адресу: г. Казань, ул. Баумана, д. 7, кв. 12", "ADDRESS", "г. Казань, ул. Баумана, д. 7, кв. 12")}},
		{Text: "Адрес проживания: г. Новосибирск, пр-т Ленина, д. 3", Expected: []Expected{exp("Адрес проживания: г. Новосибирск, пр-т Ленина, д. 3", "ADDRESS", "г. Новосибирск, пр-т Ленина, д. 3")}},
		{Text: "Клиент живет по адресу: г. Москва, ул. Тверская, д. 10, кв. 5", Expected: []Expected{exp("Клиент живет по адресу: г. Москва, ул. Тверская, д. 10, кв. 5", "ADDRESS", "г. Москва, ул. Тверская, д. 10, кв. 5")}},

		// ================= EMAIL =================
		{Text: "Email: ivanov@example.com", Expected: []Expected{exp("Email: ivanov@example.com", "EMAIL", "ivanov@example.com")}},
		{Text: "Электронная почта: petrova@mail.ru", Expected: []Expected{exp("Электронная почта: petrova@mail.ru", "EMAIL", "petrova@mail.ru")}},
		{Text: "e-mail: a.sidorov@yandex.ru", Expected: []Expected{exp("e-mail: a.sidorov@yandex.ru", "EMAIL", "a.sidorov@yandex.ru")}},

		// ================= PHONE =================
		{Text: "Телефон: +7 (912) 345-67-89", Expected: []Expected{exp("Телефон: +7 (912) 345-67-89", "PHONE", "+7 (912) 345-67-89")}},
		{Text: "Мобильный 8 912 345 67 89", Expected: []Expected{exp("Мобильный 8 912 345 67 89", "PHONE", "8 912 345 67 89")}},
		{Text: "Тел. +79123456789", Expected: []Expected{exp("Тел. +79123456789", "PHONE", "+79123456789")}},
		{Text: "Контактный телефон: 7 (495) 123-45-67", Expected: []Expected{exp("Контактный телефон: 7 (495) 123-45-67", "PHONE", "7 (495) 123-45-67")}},

		// ================= INN =================
		{Text: "ИНН 7707083893", Expected: []Expected{exp("ИНН 7707083893", "INN", "7707083893")}},
		{Text: "ИНН физического лица 500100732259", Expected: []Expected{exp("ИНН физического лица 500100732259", "INN", "500100732259")}},
		{Text: "ИНН: 7707083893", Expected: []Expected{exp("ИНН: 7707083893", "INN", "7707083893")}},

		// ================= CARD_NUMBER =================
		{Text: "Номер карты 4532 0151 1283 0366", Expected: []Expected{exp("Номер карты 4532 0151 1283 0366", "CARD_NUMBER", "4532 0151 1283 0366")}},
		{Text: "Карта 4532015112830366", Expected: []Expected{exp("Карта 4532015112830366", "CARD_NUMBER", "4532015112830366")}},
		{Text: "Номер банковской карты: 4916 1197 1130 4546", Expected: []Expected{exp("Номер банковской карты: 4916 1197 1130 4546", "CARD_NUMBER", "4916 1197 1130 4546")}},

		// ================= CVV =================
		{Text: "CVV 123", Expected: []Expected{exp("CVV 123", "CVV", "123")}},
		{Text: "Код безопасности 456", Expected: []Expected{exp("Код безопасности 456", "CVV", "456")}},
		{Text: "Номер карты 4532 0151 1283 0366, CVV 123", Expected: []Expected{exp("Номер карты 4532 0151 1283 0366, CVV 123", "CARD_NUMBER", "4532 0151 1283 0366"), exp("Номер карты 4532 0151 1283 0366, CVV 123", "CVV", "123")}},

		// ================= PIN =================
		{Text: "Пин-код 7305", Expected: []Expected{exp("Пин-код 7305", "PIN", "7305")}},
		{Text: "PIN 1234", Expected: []Expected{exp("PIN 1234", "PIN", "1234")}},
		{Text: "Номер карты 4532 0151 1283 0366, пин-код 7305", Expected: []Expected{exp("Номер карты 4532 0151 1283 0366, пин-код 7305", "CARD_NUMBER", "4532 0151 1283 0366"), exp("Номер карты 4532 0151 1283 0366, пин-код 7305", "PIN", "7305")}},

		// ================= CARDHOLDER_NAME =================
		{Text: "Держатель карты IVAN PETROV", Expected: []Expected{exp("Держатель карты IVAN PETROV", "CARDHOLDER_NAME", "IVAN PETROV")}},
		{Text: "CARDHOLDER: IVAN PETROV", Expected: []Expected{exp("CARDHOLDER: IVAN PETROV", "CARDHOLDER_NAME", "IVAN PETROV")}},
		{Text: "На карте указано ANNA SIDOROVA", Expected: []Expected{exp("На карте указано ANNA SIDOROVA", "CARDHOLDER_NAME", "ANNA SIDOROVA")}},

		// ================= MIXED CASES =================
		{Text: "Клиент Иванов Иван Петрович, дата рождения 15.03.1990, паспорт 4510 123456, ИНН 7707083893, тел. +7 (912) 345-67-89, email ivanov@example.com",
			Expected: []Expected{
				exp("Клиент Иванов Иван Петрович, дата рождения 15.03.1990, паспорт 4510 123456, ИНН 7707083893, тел. +7 (912) 345-67-89, email ivanov@example.com", "FULL_NAME", "Иванов Иван Петрович"),
				exp("Клиент Иванов Иван Петрович, дата рождения 15.03.1990, паспорт 4510 123456, ИНН 7707083893, тел. +7 (912) 345-67-89, email ivanov@example.com", "BIRTH_DATE", "15.03.1990"),
				exp("Клиент Иванов Иван Петрович, дата рождения 15.03.1990, паспорт 4510 123456, ИНН 7707083893, тел. +7 (912) 345-67-89, email ivanov@example.com", "PASSPORT", "4510 123456"),
				exp("Клиент Иванов Иван Петрович, дата рождения 15.03.1990, паспорт 4510 123456, ИНН 7707083893, тел. +7 (912) 345-67-89, email ivanov@example.com", "INN", "7707083893"),
				exp("Клиент Иванов Иван Петрович, дата рождения 15.03.1990, паспорт 4510 123456, ИНН 7707083893, тел. +7 (912) 345-67-89, email ivanov@example.com", "PHONE", "+7 (912) 345-67-89"),
				exp("Клиент Иванов Иван Петрович, дата рождения 15.03.1990, паспорт 4510 123456, ИНН 7707083893, тел. +7 (912) 345-67-89, email ivanov@example.com", "EMAIL", "ivanov@example.com"),
			}},
		{Text: "Заявитель: Петрова Анна, адрес: г. Москва, ул. Тверская, д. 10, кв. 5, гражданство РФ",
			Expected: []Expected{
				exp("Заявитель: Петрова Анна, адрес: г. Москва, ул. Тверская, д. 10, кв. 5, гражданство РФ", "FULL_NAME", "Петрова Анна"),
				exp("Заявитель: Петрова Анна, адрес: г. Москва, ул. Тверская, д. 10, кв. 5, гражданство РФ", "ADDRESS", "г. Москва, ул. Тверская, д. 10, кв. 5"),
				exp("Заявитель: Петрова Анна, адрес: г. Москва, ул. Тверская, д. 10, кв. 5, гражданство РФ", "CITIZENSHIP", "РФ"),
			}},
		{Text: "Номер карты 4532 0151 1283 0366, CVV 123, пин-код 7305, держатель IVAN PETROV",
			Expected: []Expected{
				exp("Номер карты 4532 0151 1283 0366, CVV 123, пин-код 7305, держатель IVAN PETROV", "CARD_NUMBER", "4532 0151 1283 0366"),
				exp("Номер карты 4532 0151 1283 0366, CVV 123, пин-код 7305, держатель IVAN PETROV", "CVV", "123"),
				exp("Номер карты 4532 0151 1283 0366, CVV 123, пин-код 7305, держатель IVAN PETROV", "PIN", "7305"),
				exp("Номер карты 4532 0151 1283 0366, CVV 123, пин-код 7305, держатель IVAN PETROV", "CARDHOLDER_NAME", "IVAN PETROV"),
			}},
		{Text: "Паспорт 4510 123456, выдан ОВМ УМВД России по г. Москве, код подразделения 770-001, дата выдачи 20.04.2010",
			Expected: []Expected{
				exp("Паспорт 4510 123456, выдан ОВМ УМВД России по г. Москве, код подразделения 770-001, дата выдачи 20.04.2010", "PASSPORT", "4510 123456"),
				exp("Паспорт 4510 123456, выдан ОВМ УМВД России по г. Москве, код подразделения 770-001, дата выдачи 20.04.2010", "PASSPORT_ISSUER", "ОВМ УМВД России по г. Москве"),
				exp("Паспорт 4510 123456, выдан ОВМ УМВД России по г. Москве, код подразделения 770-001, дата выдачи 20.04.2010", "DEPARTMENT_CODE", "770-001"),
				exp("Паспорт 4510 123456, выдан ОВМ УМВД России по г. Москве, код подразделения 770-001, дата выдачи 20.04.2010", "PASSPORT_ISSUE_DATE", "20.04.2010"),
			}},
		{Text: "Водительское удостоверение 7712 345678, адрес: г. Москва, ул. Тверская, д. 10, кв. 5",
			Expected: []Expected{
				exp("Водительское удостоверение 7712 345678, адрес: г. Москва, ул. Тверская, д. 10, кв. 5", "DRIVER_LICENSE", "7712 345678"),
				exp("Водительское удостоверение 7712 345678, адрес: г. Москва, ул. Тверская, д. 10, кв. 5", "ADDRESS", "г. Москва, ул. Тверская, д. 10, кв. 5"),
			}},

		// ================= ADDITIONAL FORMAT VARIATIONS =================
		{Text: "ФИО: Смирнов Пётр Иванович", Expected: []Expected{exp("ФИО: Смирнов Пётр Иванович", "FULL_NAME", "Смирнов Пётр Иванович")}},
		{Text: "Клиент: Кузнецова Анна", Expected: []Expected{exp("Клиент: Кузнецова Анна", "FULL_NAME", "Кузнецова Анна")}},
		{Text: "Дата рождения 25.12.1975", Expected: []Expected{exp("Дата рождения 25.12.1975", "BIRTH_DATE", "25.12.1975")}},
		{Text: "Родился 3 марта 1995", Expected: []Expected{exp("Родился 3 марта 1995", "BIRTH_DATE", "3 марта 1995")}},
		{Text: "Место рождения: г. Екатеринбург", Expected: []Expected{exp("Место рождения: г. Екатеринбург", "BIRTH_PLACE", "г. Екатеринбург")}},
		{Text: "Родился в Самаре", Expected: []Expected{exp("Родился в Самаре", "BIRTH_PLACE", "Самаре")}},
		{Text: "Паспорт 4509 876543", Expected: []Expected{exp("Паспорт 4509 876543", "PASSPORT", "4509 876543")}},
		{Text: "Серия 45 09 номер 876543", Expected: []Expected{exp("Серия 45 09 номер 876543", "PASSPORT", "Серия 45 09 номер 876543")}},
		{Text: "Паспорт: серия 4510, номер 123456", Expected: []Expected{exp("Паспорт: серия 4510, номер 123456", "PASSPORT", "серия 4510, номер 123456")}},
		{Text: "паспорт 4510 №123456", Expected: []Expected{exp("паспорт 4510 №123456", "PASSPORT", "4510 №123456")}},
		{Text: "паспорт 45 10 № 123456", Expected: []Expected{exp("паспорт 45 10 № 123456", "PASSPORT", "45 10 № 123456")}},
		{Text: "серия 45 10, номер 123456", Expected: []Expected{exp("серия 45 10, номер 123456", "PASSPORT", "серия 45 10, номер 123456")}},
		{Text: "Гражданство: Украина", Expected: []Expected{exp("Гражданство: Украина", "CITIZENSHIP", "Украина")}},
		{Text: "Гражданин Казахстана", Expected: []Expected{exp("Гражданин Казахстана", "CITIZENSHIP", "Казахстана")}},
		{Text: "является гражданином Армении", Expected: []Expected{exp("является гражданином Армении", "CITIZENSHIP", "Армении")}},
		{Text: "Гражданство заявителя Россия", Expected: []Expected{exp("Гражданство заявителя Россия", "CITIZENSHIP", "Россия")}},
		{Text: "Россия — страна. Гражданство клиента Россия", Expected: []Expected{{Type: entity.CITIZENSHIP, Text: "Россия", Start: 69, End: 81}}},
		{Text: "Паспорт выдан ОМВД России по г. Казани", Expected: []Expected{exp("Паспорт выдан ОМВД России по г. Казани", "PASSPORT_ISSUER", "ОМВД России по г. Казани")}},
		{Text: "Код подразделения 160-045", Expected: []Expected{exp("Код подразделения 160-045", "DEPARTMENT_CODE", "160-045")}},
		{Text: "Дата выдачи 12.08.2012", Expected: []Expected{exp("Дата выдачи 12.08.2012", "PASSPORT_ISSUE_DATE", "12.08.2012")}},
		{Text: "Водительское удостоверение 9901 234567", Expected: []Expected{exp("Водительское удостоверение 9901 234567", "DRIVER_LICENSE", "9901 234567")}},
		{Text: "Адрес: г. Санкт-Петербург, Невский пр-т, д. 15, кв. 3", Expected: []Expected{exp("Адрес: г. Санкт-Петербург, Невский пр-т, д. 15, кв. 3", "ADDRESS", "г. Санкт-Петербург, Невский пр-т, д. 15, кв. 3")}},
		{Text: "Email: smirnov@yandex.ru", Expected: []Expected{exp("Email: smirnov@yandex.ru", "EMAIL", "smirnov@yandex.ru")}},
		{Text: "Телефон: 8 (495) 123-45-67", Expected: []Expected{exp("Телефон: 8 (495) 123-45-67", "PHONE", "8 (495) 123-45-67")}},
		{Text: "ИНН 500100732259", Expected: []Expected{exp("ИНН 500100732259", "INN", "500100732259")}},
		{Text: "Номер карты 4485 2757 4230 8327", Expected: []Expected{exp("Номер карты 4485 2757 4230 8327", "CARD_NUMBER", "4485 2757 4230 8327")}},
		{Text: "CVV 789", Expected: []Expected{exp("CVV 789", "CVV", "789")}},
		{Text: "Пин-код 4321", Expected: []Expected{exp("Пин-код 4321", "PIN", "4321")}},
		{Text: "Держатель карты PETROV IVAN", Expected: []Expected{exp("Держатель карты PETROV IVAN", "CARDHOLDER_NAME", "PETROV IVAN")}},

		// ================= MORE MIXED CASES =================
		{Text: "Клиент: Волков Дмитрий, дата рождения 10.10.1980, паспорт 4510 123456, выдан ОВМ УМВД России по г. Москве, код подразделения 770-001, ИНН 7707083893, тел. +7 (912) 345-67-89, email volkov@example.com, адрес: г. Москва, ул. Тверская, д. 10, кв. 5",
			Expected: []Expected{
				exp("Клиент: Волков Дмитрий, дата рождения 10.10.1980, паспорт 4510 123456, выдан ОВМ УМВД России по г. Москве, код подразделения 770-001, ИНН 7707083893, тел. +7 (912) 345-67-89, email volkov@example.com, адрес: г. Москва, ул. Тверская, д. 10, кв. 5", "FULL_NAME", "Волков Дмитрий"),
				exp("Клиент: Волков Дмитрий, дата рождения 10.10.1980, паспорт 4510 123456, выдан ОВМ УМВД России по г. Москве, код подразделения 770-001, ИНН 7707083893, тел. +7 (912) 345-67-89, email volkov@example.com, адрес: г. Москва, ул. Тверская, д. 10, кв. 5", "BIRTH_DATE", "10.10.1980"),
				exp("Клиент: Волков Дмитрий, дата рождения 10.10.1980, паспорт 4510 123456, выдан ОВМ УМВД России по г. Москве, код подразделения 770-001, ИНН 7707083893, тел. +7 (912) 345-67-89, email volkov@example.com, адрес: г. Москва, ул. Тверская, д. 10, кв. 5", "PASSPORT", "4510 123456"),
				exp("Клиент: Волков Дмитрий, дата рождения 10.10.1980, паспорт 4510 123456, выдан ОВМ УМВД России по г. Москве, код подразделения 770-001, ИНН 7707083893, тел. +7 (912) 345-67-89, email volkov@example.com, адрес: г. Москва, ул. Тверская, д. 10, кв. 5", "PASSPORT_ISSUER", "ОВМ УМВД России по г. Москве"),
				exp("Клиент: Волков Дмитрий, дата рождения 10.10.1980, паспорт 4510 123456, выдан ОВМ УМВД России по г. Москве, код подразделения 770-001, ИНН 7707083893, тел. +7 (912) 345-67-89, email volkov@example.com, адрес: г. Москва, ул. Тверская, д. 10, кв. 5", "DEPARTMENT_CODE", "770-001"),
				exp("Клиент: Волков Дмитрий, дата рождения 10.10.1980, паспорт 4510 123456, выдан ОВМ УМВД России по г. Москве, код подразделения 770-001, ИНН 7707083893, тел. +7 (912) 345-67-89, email volkov@example.com, адрес: г. Москва, ул. Тверская, д. 10, кв. 5", "INN", "7707083893"),
				exp("Клиент: Волков Дмитрий, дата рождения 10.10.1980, паспорт 4510 123456, выдан ОВМ УМВД России по г. Москве, код подразделения 770-001, ИНН 7707083893, тел. +7 (912) 345-67-89, email volkov@example.com, адрес: г. Москва, ул. Тверская, д. 10, кв. 5", "PHONE", "+7 (912) 345-67-89"),
				exp("Клиент: Волков Дмитрий, дата рождения 10.10.1980, паспорт 4510 123456, выдан ОВМ УМВД России по г. Москве, код подразделения 770-001, ИНН 7707083893, тел. +7 (912) 345-67-89, email volkov@example.com, адрес: г. Москва, ул. Тверская, д. 10, кв. 5", "EMAIL", "volkov@example.com"),
				exp("Клиент: Волков Дмитрий, дата рождения 10.10.1980, паспорт 4510 123456, выдан ОВМ УМВД России по г. Москве, код подразделения 770-001, ИНН 7707083893, тел. +7 (912) 345-67-89, email volkov@example.com, адрес: г. Москва, ул. Тверская, д. 10, кв. 5", "ADDRESS", "г. Москва, ул. Тверская, д. 10, кв. 5"),
			}},
		{Text: "Гражданство: Российская Федерация, паспорт 4510 123456, дата рождения 15.03.1990",
			Expected: []Expected{
				exp("Гражданство: Российская Федерация, паспорт 4510 123456, дата рождения 15.03.1990", "CITIZENSHIP", "Российская Федерация"),
				exp("Гражданство: Российская Федерация, паспорт 4510 123456, дата рождения 15.03.1990", "PASSPORT", "4510 123456"),
				exp("Гражданство: Российская Федерация, паспорт 4510 123456, дата рождения 15.03.1990", "BIRTH_DATE", "15.03.1990"),
			}},

		// ================= HARD NEGATIVES =================
		// Known persons in literary/historical context.
		{Text: "Александр Сергеевич Пушкин — великий русский поэт.", Negative: true},
		{Text: "Лев Николаевич Толстой написал роман «Война и мир».", Negative: true},
		{Text: "Поэт Александр Сергеевич Пушкин родился в Москве.", Negative: true},
		{Text: "Владимир Путин — президент России.", Negative: true},
		{Text: "Юрий Гагарин — первый космонавт.", Negative: true},
		{Text: "Пётр Чайковский — великий русский композитор.", Negative: true},
		// Public/bank branch addresses.
		{Text: "Отделение банка находится по адресу: г. Москва, ул. Тверская, д. 1.", Negative: true},
		{Text: "Офис банка расположен по адресу: г. Казань, ул. Баумана, д. 5.", Negative: true},
		{Text: "Магазин находится по адресу: г. Москва, ул. Тверская, д. 1.", Negative: true},
		{Text: "Адрес ресторана: г. Москва, ул. Тверская, д. 1.", Negative: true},
		// Order/article/document numbers that are not passports.
		{Text: "Номер заказа 1234567890123456.", Negative: true},
		{Text: "Номер заказа 1234567890.", Negative: true},
		{Text: "Артикул 4510123456.", Negative: true},
		{Text: "Артикул 7707083893.", Negative: true},
		{Text: "Артикул 500100732259.", Negative: true},
		{Text: "Номер документа 4510 123456.", Negative: true},
		{Text: "Номер накладной 1234567890.", Negative: true},
		{Text: "Трек-номер 1234567890.", Negative: true},
		{Text: "Номер договора 1234567890.", Negative: true},
		{Text: "Счёт-фактура 1234567890.", Negative: true},
		{Text: "Квитанция 1234567890.", Negative: true},
		{Text: "Номер заявки 1234567890.", Negative: true},
		{Text: "Номер партии 1234567890.", Negative: true},
		{Text: "Серийный номер 4510123456.", Negative: true},
		{Text: "Номер счёта 1234567890.", Negative: true},
		// Department codes without passport context.
		{Text: "Код 770-001 указан в заявке.", Negative: true},
		{Text: "Код товара 770-001.", Negative: true},
		{Text: "Партия товара 770-001.", Negative: true},
		// PIN/CVV without banking context.
		{Text: "Код доступа 7305.", Negative: true},
		{Text: "Пароль 1234.", Negative: true},
		{Text: "Код подтверждения 1234.", Negative: true},
		{Text: "Код клиента 1234.", Negative: true},
		// Auditorium/room numbers that are not CVV.
		{Text: "Лекция пройдёт в аудитории 314.", Negative: true},
		{Text: "Кабинет 314.", Negative: true},
		{Text: "Комната 314.", Negative: true},
		{Text: "Офис 314.", Negative: true},
		{Text: "Помещение 314.", Negative: true},
		// Event dates that are not birth dates.
		{Text: "Конференция состоится 15.03.2025.", Negative: true},
		{Text: "Встреча назначена на 20.04.2025 в 15:00.", Negative: true},
		{Text: "Дата рождения 31.02.2000.", Negative: true},
		// Bare country mentions that are not citizenship.
		{Text: "Россия — крупнейшая страна мира.", Negative: true},
		{Text: "Германия — страна в центре Европы.", Negative: true},
		// Metro/transport cards that are not bank cards.
		{Text: "Карта метро 4532 0151 1283 0366.", Negative: true},
		{Text: "Транспортная карта 4916 1197 1130 4546.", Negative: true},
		// Public organization contacts.
		{Text: "Служба поддержки: 8 800 123 45 67.", Negative: true},
		{Text: "Горячая линия: +7 (495) 123-45-67.", Negative: true},
		{Text: "Наш email: support@example.com.", Negative: true},
		{Text: "Обращайтесь по телефону 8 800 555 35 35.", Negative: true},
		{Text: "Телефон офиса: +7 (495) 123-45-67.", Negative: true},
		{Text: "Email компании: info@company.ru.", Negative: true},
		{Text: "Email отдела продаж: sales@company.ru.", Negative: true},
		// Cardholder without banking context.
		{Text: "hello world", Negative: true},
		{Text: "Open AI platform", Negative: true},
		// Loyalty/transport cards that are not bank cards.
		{Text: "Карта лояльности 4111 1111 1111 1111.", Negative: true},
		// Non-bank CVV/PIN.
		{Text: "Код безопасности 123 для замка.", Negative: true},
		{Text: "PIN 1234 от телефона.", Negative: true},
		{Text: "код 123 от замка.", Negative: true},
		{Text: "код безопасности 123 для сейфа.", Negative: true},
		{Text: "PIN SIM-карты 1234.", Negative: true},
		{Text: "код роутера 1234.", Negative: true},
		// Department codes without passport context.
		{Text: "код заявки 770-001.", Negative: true},
		// Bare driver license / passport numbers without context.
		{Text: "77 12 345678", Negative: true},
		{Text: "7712 345678", Negative: true},
		{Text: "4510 123456", Negative: true},
		// Public office address.
		{Text: "Адрес офиса: г. Москва, ул. Тверская, д. 1.", Negative: true},
	}
}
