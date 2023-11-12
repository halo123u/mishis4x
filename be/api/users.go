package api

type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Status   string `json:"status"`
}

type UserLogin struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
