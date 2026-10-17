package proxy

import "time"

func (handler *activeRequestHandler) waitBudget(lane admissionLane, pressure bool) time.Duration {
	longLived := lane == admissionLaneResponses || lane == admissionLaneWebSocket
	if longLived {
		if pressure && handler.pressureLongLivedQueueWaitBudget > 0 {
			return handler.pressureLongLivedQueueWaitBudget
		}
		if handler.longLivedQueueWaitBudget > 0 {
			return handler.longLivedQueueWaitBudget
		}
	} else {
		if pressure && handler.pressureAdmissionWaitBudget > 0 {
			return handler.pressureAdmissionWaitBudget
		}
		if handler.admissionWaitBudget > 0 {
			return handler.admissionWaitBudget
		}
	}
	if pressure {
		return pressureAdmissionWaitInterval
	}
	return admissionWaitInterval
}
