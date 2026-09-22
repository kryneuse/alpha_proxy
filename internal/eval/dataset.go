// Package eval provides an evaluation dataset and metrics computation for the
// rule engine. All data is synthetic.
package eval

import "github.com/alpha-proxy/rule-engine/internal/entity"

// Sample is a single evaluation case.
type Sample struct {
	Text     string
	Expected []entity.Type
	// Negative marks a hard-negative case (no entities expected).
	Negative bool
}

// Dataset returns the evaluation dataset.
func Dataset() []Sample {
	return []Sample{
		// --- Positive cases for all 17 types ---
		{Text: "Клиент: Иванов Иван Петрович", Expected: []entity.Type{entity.FULL_NAME}},
		{Text: "ФИО заявителя: Петрова Анна Сергеевна", Expected: []entity.Type{entity.FULL_NAME}},
		{Text: "Дата рождения: 15.03.1990", Expected: []entity.Type{entity.BIRTH_DATE}},
		{Text: "Родился 01-02-2000", Expected: []entity.Type{entity.BIRTH_DATE}},
		{Text: "Дата рождения 12 января 1985", Expected: []entity.Type{entity.BIRTH_DATE}},
		{Text: "Место рождения: город Москва", Expected: []entity.Type{entity.BIRTH_PLACE}},
		{Text: "Родился в городе Санкт-Петербург", Expected: []entity.Type{entity.BIRTH_PLACE}},
		{Text: "Паспорт 4510 123456", Expected: []entity.Type{entity.PASSPORT}},
		{Text: "Серия 45 10 номер 123456", Expected: []entity.Type{entity.PASSPORT}},
		{Text: "Гражданство: Российская Федерация", Expected: []entity.Type{entity.CITIZENSHIP}},
		{Text: "Гражданин РФ", Expected: []entity.Type{entity.CITIZENSHIP}},
		{Text: "Паспорт выдан ОВМ УМВД России по г. Москве", Expected: []entity.Type{entity.PASSPORT_ISSUER}},
		{Text: "Код подразделения 770-001", Expected: []entity.Type{entity.DEPARTMENT_CODE}},
		{Text: "Паспорт выдан 20.04.2010", Expected: []entity.Type{entity.PASSPORT_ISSUE_DATE}},
		{Text: "Дата выдачи: 05.11.2015", Expected: []entity.Type{entity.PASSPORT_ISSUE_DATE}},
		{Text: "Водительское удостоверение 7712 345678", Expected: []entity.Type{entity.DRIVER_LICENSE}},
		{Text: "Адрес: г. Москва, ул. Тверская, д. 10, кв. 5", Expected: []entity.Type{entity.ADDRESS}},
		{Text: "Адрес регистрации: 101000, г. Москва, ул. Арбат, д. 1", Expected: []entity.Type{entity.ADDRESS}},
		{Text: "Email: ivanov@example.com", Expected: []entity.Type{entity.EMAIL}},
		{Text: "Телефон: +7 (912) 345-67-89", Expected: []entity.Type{entity.PHONE}},
		{Text: "Мобильный 8 912 345 67 89", Expected: []entity.Type{entity.PHONE}},
		{Text: "ИНН 7707083893", Expected: []entity.Type{entity.INN}},
		{Text: "ИНН физического лица 500100732259", Expected: []entity.Type{entity.INN}},
		{Text: "Номер карты 4532 0151 1283 0366", Expected: []entity.Type{entity.CARD_NUMBER}},
		{Text: "Карта 4532015112830366", Expected: []entity.Type{entity.CARD_NUMBER}},
		{Text: "CVV 123", Expected: []entity.Type{entity.CVV}},
		{Text: "Код безопасности 456", Expected: []entity.Type{entity.CVV}},
		{Text: "Пин-код 7305", Expected: []entity.Type{entity.PIN}},
		{Text: "PIN 1234", Expected: []entity.Type{entity.PIN}},
		{Text: "Держатель карты IVAN PETROV", Expected: []entity.Type{entity.CARDHOLDER_NAME}},
		{Text: "CARDHOLDER: IVAN PETROV", Expected: []entity.Type{entity.CARDHOLDER_NAME}},

		// --- Mixed cases with multiple PD ---
		{Text: "Клиент Иванов Иван Петрович, дата рождения 15.03.1990, паспорт 4510 123456, ИНН 7707083893, тел. +7 (912) 345-67-89, email ivanov@example.com",
			Expected: []entity.Type{entity.FULL_NAME, entity.BIRTH_DATE, entity.PASSPORT, entity.INN, entity.PHONE, entity.EMAIL}},
		{Text: "Заявитель: Петрова Анна, адрес: г. Москва, ул. Тверская, д. 10, кв. 5, гражданство РФ",
			Expected: []entity.Type{entity.FULL_NAME, entity.ADDRESS, entity.CITIZENSHIP}},
		{Text: "Номер карты 4532 0151 1283 0366, CVV 123, пин-код 7305, держатель IVAN PETROV",
			Expected: []entity.Type{entity.CARD_NUMBER, entity.CVV, entity.PIN, entity.CARDHOLDER_NAME}},

		// --- Hard negatives ---
		{Text: "Александр Сергеевич Пушкин — великий русский поэт.", Negative: true},
		{Text: "Отделение банка находится по адресу: г. Москва, ул. Тверская, д. 1.", Negative: true},
		{Text: "Номер заказа 1234567890123456.", Negative: true},
		{Text: "Код доступа 7305.", Negative: true},
		{Text: "Лекция пройдёт в аудитории 314.", Negative: true},
		{Text: "Конференция состоится 15.03.2025.", Negative: true},
		{Text: "Встреча назначена на 20.04.2025 в 15:00.", Negative: true},
		{Text: "Код товара 770-001.", Negative: true},
	}
}
