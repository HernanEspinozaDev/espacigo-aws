package main

import (
	"net/http"

	commercialcommunityhttp "github.com/HernanEspinozaDev/espaciGo/internal/commercialcommunity/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/conversation"
	"github.com/HernanEspinozaDev/espaciGo/internal/reputation"
)

func registerCommercialCommunityRoutes(mux *http.ServeMux, auth commercialcommunityhttp.Authenticator, conversations *conversation.Service, reviews *reputation.Service, origins []string) {
	handler := commercialcommunityhttp.NewHandler(auth, conversations, reviews, origins)
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodOptions} {
		mux.Handle(method+" /api/v1/commercial-reservations/{reservationID}/messages", handler)
	}
	for _, method := range []string{http.MethodPost, http.MethodOptions} {
		mux.Handle(method+" /api/v1/commercial-reservations/{reservationID}/messages/read", handler)
	}
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodOptions} {
		mux.Handle(method+" /api/v1/commercial-reservations/{reservationID}/reviews", handler)
	}
}
