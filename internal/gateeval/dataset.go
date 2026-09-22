// Package gateeval provides an evaluation dataset and metrics for the
// heuristic cheap gate. The question answered is: "does the residual text
// still contain PII?" Labels are binary: 0 = clean, 1 = contains PII.
package gateeval

// Sample is a single gate evaluation case.
type Sample struct {
	// Text is the residual text to evaluate.
	Text string
	// HasPII is 1 if the residual contains at least one PII, 0 otherwise.
	HasPII int
}

// Dataset returns the gate evaluation dataset. It is intentionally small and
// diverse, focusing on residuals that the strict rule engine might miss.
func Dataset() []Sample {
	return []Sample{
		// --- Clean residuals ---
		{Text: "сегодня хорошая погода на улице", HasPII: 0},
		{Text: "конференция состоится в пятницу", HasPII: 0},
		{Text: "мы обсудили план на следующий квартал", HasPII: 0},
		{Text: "отчёт готов и отправлен руководству", HasPII: 0},
		{Text: "встреча назначена на десять утра", HasPII: 0},
		{Text: "проект завершён в срок", HasPII: 0},
		{Text: "команда работает над новой версией", HasPII: 0},
		{Text: "документы подписаны", HasPII: 0},
		{Text: "заседание перенесено на следующую неделю", HasPII: 0},
		{Text: "результаты тестирования положительные", HasPII: 0},
		{Text: "мы получили ваше письмо", HasPII: 0},
		{Text: "благодарим за обращение", HasPII: 0},
		{Text: "заявка принята в обработку", HasPII: 0},
		{Text: "статус заказа обновлён", HasPII: 0},
		{Text: "оплата прошла успешно", HasPII: 0},
		{Text: "доставка будет завтра", HasPII: 0},
		{Text: "номер вашего обращения 12345", HasPII: 0},
		{Text: "код подтверждения отправлен", HasPII: 0},
		{Text: "мы свяжемся с вами позже", HasPII: 0},
		{Text: "спасибо за покупку", HasPII: 0},

		// --- Residuals with PII the strict rule engine might miss ---
		{Text: "4510 123456", HasPII: 1},
		{Text: "паспорт 4510 123456", HasPII: 1},
		{Text: "серия 4510 номер 123456", HasPII: 1},
		{Text: "7712 345678", HasPII: 1},
		{Text: "водительское удостоверение 77 12 345678", HasPII: 1},
		{Text: "7707083893", HasPII: 1},
		{Text: "ИНН 7707083893", HasPII: 1},
		{Text: "4532 0151 1283 0366", HasPII: 1},
		{Text: "номер карты 4532 0151 1283 0366", HasPII: 1},
		{Text: "+7 (912) 345-67-89", HasPII: 1},
		{Text: "телефон +7 (912) 345-67-89", HasPII: 1},
		{Text: "ivanov@example.com", HasPII: 1},
		{Text: "email ivanov@example.com", HasPII: 1},
		{Text: "15.03.1990", HasPII: 1},
		{Text: "дата рождения 15.03.1990", HasPII: 1},
		{Text: "770-001", HasPII: 1},
		{Text: "код подразделения 770-001", HasPII: 1},
		{Text: "Иван Петров", HasPII: 1},
		{Text: "клиент Иван Петров", HasPII: 1},
		{Text: "Смирнова Анна Сергеевна", HasPII: 1},
		{Text: "заявитель Смирнова Анна", HasPII: 1},
		{Text: "проживает по адресу г. Москва ул. Тверская д. 10", HasPII: 1},
		{Text: "адрес регистрации г. Казань ул. Баумана д. 7", HasPII: 1},
		{Text: "гражданство Российская Федерация", HasPII: 1},
		{Text: "гражданин Казахстана", HasPII: 1},
		{Text: "родился в городе Санкт-Петербург", HasPII: 1},
		{Text: "место рождения город Москва", HasPII: 1},
		{Text: "CVV 123", HasPII: 1},
		{Text: "пин-код 7305", HasPII: 1},
		{Text: "держатель карты IVAN PETROV", HasPII: 1},
		{Text: "паспорт выдан ОВМ УМВД России", HasPII: 1},
		{Text: "код подразделения 452-013", HasPII: 1},
		{Text: "водительские права 9901 234567", HasPII: 1},
		{Text: "ИНН физического лица 500100732259", HasPII: 1},
		{Text: "номер счёта 40817810099910004312", HasPII: 1},
		{Text: "паспорт 45 10 № 123456", HasPII: 1},
		{Text: "серия 45 10, номер 123456", HasPII: 1},
		{Text: "является гражданином Армении", HasPII: 1},
		{Text: "Гражданка Республики Беларусь", HasPII: 1},

		// --- Weak/implicit context ---
		{Text: "родился в 1990 году", HasPII: 1},
		{Text: "дата рождения 12 января 1985", HasPII: 1},
		{Text: "живет по адресу г. Новосибирск", HasPII: 1},
		{Text: "зарегистрирован по адресу г. Екатеринбург", HasPII: 1},
		{Text: "клиент Кузнецова Мария", HasPII: 1},
		{Text: "пациент Волков Дмитрий", HasPII: 1},
		{Text: "заёмщик Сидоров Алексей", HasPII: 1},
		{Text: "держатель полиса Петрова Анна", HasPII: 1},

		// --- Unknown names / free-form addresses ---
		{Text: "Абдуллаев Тимур Рашидович", HasPII: 1},
		{Text: "клиент Абдуллаев Тимур", HasPII: 1},
		{Text: "проживает в деревне Малые Ключи", HasPII: 1},
		{Text: "адрес: посёлок Сосновый Бор, улица Лесная", HasPII: 1},
		{Text: "родился в селе Верхние Поляны", HasPII: 1},

		// --- Document-like values without explicit label ---
		{Text: "1234 567890", HasPII: 1},
		{Text: "9876 543210", HasPII: 1},
		{Text: "123456789012", HasPII: 1},
		{Text: "1234567890", HasPII: 1},
		{Text: "4111 1111 1111 1111", HasPII: 1},
		{Text: "5555 5555 5555 4444", HasPII: 1},

		// --- Ambiguous numbers ---
		{Text: "код 123", HasPII: 1},
		{Text: "номер 4567", HasPII: 1},
		{Text: "пароль 1234", HasPII: 1},
		{Text: "код доступа 7305", HasPII: 1},

		// --- Hard negatives / public data ---
		{Text: "офис компании находится по адресу г. Москва ул. Тверская д. 1", HasPII: 0},
		{Text: "магазин расположен по адресу г. Казань ул. Баумана д. 5", HasPII: 0},
		{Text: "служба поддержки 8 800 123 45 67", HasPII: 0},
		{Text: "горячая линия +7 (495) 123-45-67", HasPII: 0},
		{Text: "наш email support@example.com", HasPII: 0},
		{Text: "отдел продаж sales@company.ru", HasPII: 0},
		{Text: "Россия — крупнейшая страна мира", HasPII: 0},
		{Text: "Германия — страна в центре Европы", HasPII: 0},
		{Text: "Летим в Казахстан", HasPII: 0},
		{Text: "карта метро 4532 0151 1283 0366", HasPII: 0},
		{Text: "транспортная карта 4916 1197 1130 4546", HasPII: 0},
		{Text: "номер заказа 1234567890", HasPII: 0},
		{Text: "артикул 4510123456", HasPII: 0},
		{Text: "код товара 770-001", HasPII: 0},
		{Text: "код заявки 770-001", HasPII: 0},
		{Text: "аудитория 314", HasPII: 0},
		{Text: "кабинет 314", HasPII: 0},
		{Text: "конференция состоится 15.03.2025", HasPII: 0},
		{Text: "встреча назначена на 20.04.2025", HasPII: 0},
		{Text: "Александр Сергеевич Пушкин — великий русский поэт", HasPII: 0},
		{Text: "Лев Николаевич Толстой написал роман", HasPII: 0},
		{Text: "Владимир Путин — президент России", HasPII: 0},
		{Text: "Юрий Гагарин — первый космонавт", HasPII: 0},
		{Text: "Пётр Чайковский — великий композитор", HasPII: 0},
		{Text: "код 123 от замка", HasPII: 0},
		{Text: "PIN 1234 от телефона", HasPII: 0},
		{Text: "код роутера 1234", HasPII: 0},
	}
}
