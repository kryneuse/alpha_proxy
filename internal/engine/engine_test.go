package engine

import (
	"testing"

	"github.com/alpha-proxy/rule-engine/internal/entity"
)

func hasEntity(entities []entity.Entity, typ entity.Type) bool {
	for _, e := range entities {
		if e.Type == typ {
			return true
		}
	}
	return false
}

func TestPipelineMixed(t *testing.T) {
	e := New(Options{})
	text := "Клиент: Иванов Иван Петрович, дата рождения 15.03.1990, " +
		"паспорт 4510 123456, выдан ОВМ УМВД России по г. Москве, " +
		"код подразделения 770-001, ИНН 7707083893, " +
		"тел. +7 (912) 345-67-89, email ivanov@example.com, " +
		"адрес: г. Москва, ул. Тверская, д. 10, кв. 5"
	entities := e.Analyze(text)

	for _, typ := range []entity.Type{
		entity.FULL_NAME, entity.BIRTH_DATE, entity.PASSPORT, entity.PASSPORT_ISSUER,
		entity.DEPARTMENT_CODE, entity.INN, entity.PHONE, entity.EMAIL, entity.ADDRESS,
	} {
		if !hasEntity(entities, typ) {
			t.Errorf("expected entity %s in %v", typ, entities)
		}
	}
}

func TestPipelineCardContext(t *testing.T) {
	e := New(Options{})
	text := "Номер карты 4532 0151 1283 0366, CVV 123, пин-код 7305, " +
		"держатель карты IVAN PETROV"
	entities := e.Analyze(text)

	for _, typ := range []entity.Type{
		entity.CARD_NUMBER, entity.CVV, entity.PIN, entity.CARDHOLDER_NAME,
	} {
		if !hasEntity(entities, typ) {
			t.Errorf("expected entity %s in %v", typ, entities)
		}
	}
}

func TestHardNegativeKnownPerson(t *testing.T) {
	e := New(Options{})
	entities := e.Analyze("Александр Сергеевич Пушкин — великий русский поэт.")
	if hasEntity(entities, entity.FULL_NAME) {
		t.Errorf("known person should not be FULL_NAME, got %v", entities)
	}
}

func TestHardNegativeBankAddress(t *testing.T) {
	e := New(Options{})
	entities := e.Analyze("Отделение банка находится по адресу: г. Москва, ул. Тверская, д. 1.")
	// The bank branch address should not be a personal ADDRESS.
	if hasEntity(entities, entity.ADDRESS) {
		t.Errorf("bank branch address should not be personal ADDRESS, got %v", entities)
	}
}

func TestHardNegativeOrderNumber(t *testing.T) {
	e := New(Options{})
	entities := e.Analyze("Номер заказа 1234567890123456.")
	if hasEntity(entities, entity.CARD_NUMBER) {
		t.Errorf("order number should not be CARD_NUMBER, got %v", entities)
	}
}

func TestHardNegativePinCode(t *testing.T) {
	e := New(Options{})
	entities := e.Analyze("Код доступа 7305.")
	if hasEntity(entities, entity.PIN) {
		t.Errorf("access code should not be PIN, got %v", entities)
	}
}

func TestHardNegativeAuditorium(t *testing.T) {
	e := New(Options{})
	entities := e.Analyze("Лекция пройдёт в аудитории 314.")
	if hasEntity(entities, entity.CVV) {
		t.Errorf("auditorium should not be CVV, got %v", entities)
	}
}

func TestHardNegativeEventDate(t *testing.T) {
	e := New(Options{})
	entities := e.Analyze("Конференция состоится 15.03.2025.")
	if hasEntity(entities, entity.BIRTH_DATE) {
		t.Errorf("event date should not be BIRTH_DATE, got %v", entities)
	}
}

func TestOffsetsPreserved(t *testing.T) {
	e := New(Options{})
	text := "Клиент: Иванов Иван Петрович"
	entities := e.Analyze(text)
	if len(entities) == 0 {
		t.Fatal("expected a name entity")
	}
	ent := entities[0]
	if text[ent.Start:ent.End] != ent.Text {
		t.Errorf("offset mismatch: text[%d:%d]=%q != %q", ent.Start, ent.End, text[ent.Start:ent.End], ent.Text)
	}
}

func TestBirthDateVsIssueDate(t *testing.T) {
	e := New(Options{})
	text := "Дата рождения 15.03.1990, паспорт выдан 20.04.2010"
	entities := e.Analyze(text)
	if !hasEntity(entities, entity.BIRTH_DATE) {
		t.Errorf("expected BIRTH_DATE, got %v", entities)
	}
	if !hasEntity(entities, entity.PASSPORT_ISSUE_DATE) {
		t.Errorf("expected PASSPORT_ISSUE_DATE, got %v", entities)
	}
}
