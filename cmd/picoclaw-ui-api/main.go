package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/sipeed/picoclaw/internal/app/catalog"
	"github.com/sipeed/picoclaw/internal/app/chat"
	"github.com/sipeed/picoclaw/internal/app/googleauth"
	"github.com/sipeed/picoclaw/internal/app/modelcatalog"
	"github.com/sipeed/picoclaw/internal/app/openaiauth"
	"github.com/sipeed/picoclaw/internal/app/qwenauth"
	"github.com/sipeed/picoclaw/internal/app/welcome"
	uihttp "github.com/sipeed/picoclaw/internal/uiapi/http"
)

const defaultAddr = "127.0.0.1:18801"

func main() {
	addr := flag.String("addr", defaultAddr, "listen address")
	flag.Parse()

	mux := http.NewServeMux()

	catalogService := catalog.NewService()
	catalogHandler := uihttp.NewCatalogHandler(catalogService)
	catalogHandler.Register(mux)

	chatService := chat.NewService()
	chatHandler := uihttp.NewChatHandler(chatService)
	chatHandler.Register(mux)

	modelCatalogService := modelcatalog.NewService()
	modelCatalogHandler := uihttp.NewProviderModelsHandler(modelCatalogService)
	modelCatalogHandler.Register(mux)

	openAIAuthService := openaiauth.NewService()
	openAIAuthHandler := uihttp.NewOpenAIAuthHandler(openAIAuthService)
	openAIAuthHandler.Register(mux)

	qwenAuthService := qwenauth.NewService()
	qwenAuthHandler := uihttp.NewQwenAuthHandler(qwenAuthService)
	qwenAuthHandler.Register(mux)

	googleAuthService := googleauth.NewService()
	googleAuthHandler := uihttp.NewGoogleAuthHandler(googleAuthService)
	googleAuthHandler.Register(mux)

	welcomeService := welcome.NewService()
	welcomeHandler := uihttp.NewWelcomeHandler(welcomeService)
	welcomeHandler.Register(mux)

	fmt.Printf("PicoClaw UI API listening on http://%s\n", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}
