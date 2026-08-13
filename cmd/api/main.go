package main

import "net/http"

func Main() {
	port := ":3000"

	mux := http.NewServeMux()

	http.ListenAndServe(port, mux)

}
