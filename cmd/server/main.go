package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/jmechavez/vapeshop/internal/auth"
	"github.com/jmechavez/vapeshop/internal/database"
	appmw "github.com/jmechavez/vapeshop/internal/middleware"
	"github.com/jmechavez/vapeshop/internal/products"
	"github.com/jmechavez/vapeshop/views/pages"
	"github.com/jmechavez/vapeshop/views/partials"
)

func main() {
	ctx := context.Background()

	if err := database.Connect(ctx); err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer database.Close()

	sessionStore := auth.NewStore()

	authHandler := &auth.Handler{
		Store: sessionStore,
		RenderLogin: func(w http.ResponseWriter, r *http.Request, errMsg string) {
			pages.Login(errMsg).Render(r.Context(), w)
		},
	}

	productsHandler := &products.Handler{
		RenderList: func(w http.ResponseWriter, r *http.Request, list []products.ProductWithStock, q string) {
			sess := auth.FromContext(r.Context())
			pages.Products(sess, list, q).Render(r.Context(), w)
		},
		RenderSearch: func(w http.ResponseWriter, r *http.Request, list []products.ProductWithStock) {
			partials.ProductsTable(list).Render(r.Context(), w)
		},
		RenderNewModal: func(w http.ResponseWriter, r *http.Request, p products.Product, errMsg string) {
			partials.ProductFormModal(p, errMsg).Render(r.Context(), w)
		},
		RenderRow: func(w http.ResponseWriter, r *http.Request, p products.ProductWithStock) {
			partials.ProductRow(p).Render(r.Context(), w)
		},
		RenderRowEdit: func(w http.ResponseWriter, r *http.Request, p products.ProductWithStock, errMsg string) {
			partials.ProductRowEdit(p, errMsg).Render(r.Context(), w)
		},
		RenderOOBTable: func(w http.ResponseWriter, r *http.Request, list []products.ProductWithStock) {
			partials.ProductsTableOOB(list).Render(r.Context(), w)
		},
		RenderImport: func(w http.ResponseWriter, r *http.Request, list []products.ProductWithStock, created int, failed []string) {
			partials.ProductsTableWithImportReport(list, created, failed).Render(r.Context(), w)
		},
	}

	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(appmw.Logger)
	r.Use(appmw.Recover)

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Public
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		pages.Home().Render(r.Context(), w)
	})
	r.Get("/login", authHandler.LoginPage)
	r.Post("/login", authHandler.Login)
	r.Post("/logout", authHandler.Logout)

	// Protected
	r.Group(func(pr chi.Router) {
		pr.Use(sessionStore.RequireLogin)

		pr.Get("/dashboard", func(w http.ResponseWriter, r *http.Request) {
			sess := auth.FromContext(r.Context())
			pages.Dashboard(sess).Render(r.Context(), w)
		})

		pr.Route("/products", func(pr chi.Router) {
			pr.Get("/", productsHandler.Index)
			pr.Get("/search", productsHandler.Search)
			pr.Get("/new", productsHandler.NewModal)
			pr.Post("/", productsHandler.Create)
			pr.Post("/import", productsHandler.ImportCSV)
			pr.Get("/{id}/edit", productsHandler.EditInline)
			pr.Get("/{id}/row", productsHandler.CancelEdit)
			pr.Put("/{id}", productsHandler.Update)
			pr.Delete("/{id}", productsHandler.Delete)
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	log.Printf("Drift Culture listening on :%s", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
