package quota

type RouteKind uint8

const (
	RouteKindStandard RouteKind = iota
	RouteKindResponses
	RouteKindCompact
	RouteKindWebSocket
)

type Selection struct {
	RouteKind      RouteKind
	RequestedModel string
}
