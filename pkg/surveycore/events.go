package surveycore

import (
	"time"

	"github.com/SurveyController/SurveyCore/internal/engine"
)

type EventHandler func(Event)

type Event struct {
	Worker  string    `json:"worker"`
	Message string    `json:"message"`
	Success bool      `json:"success"`
	Fail    bool      `json:"fail"`
	Current int       `json:"current"`
	Total   int       `json:"total"`
	Time    time.Time `json:"time"`
}

func mapEvent(event engine.StatusEvent) Event {
	return Event{
		Worker:  event.ThreadName,
		Message: event.StatusText,
		Success: event.Success,
		Fail:    event.Fail,
		Current: event.Current,
		Total:   event.Total,
		Time:    event.Timestamp,
	}
}
