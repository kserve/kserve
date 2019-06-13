package tfmodel

/** Node to be visited by the Visitor in the Visitor Design Pattern **/
type Node interface {
	Accept(w Visitor)
}
