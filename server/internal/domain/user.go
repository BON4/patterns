package domain

type User struct {
	UUID         string
	IP           string
	RequestCount int64
}

type UserRequests struct {
	IP    string
	Count int
}
