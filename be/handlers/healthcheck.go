package handlers

import "net/http"

func (d *Data)Healthcheck(w http.ResponseWriter, r *http.Request){
	 w.WriteHeader(http.StatusOK)
}