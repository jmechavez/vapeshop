// cmd/server/main.go
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
	"github.com/jmechavez/vapeshop/internal/inventory"
	appmw "github.com/jmechavez/vapeshop/internal/middleware"
	"github.com/jmechavez/vapeshop/internal/products"
	"github.com/jmechavez/vapeshop/internal/suppliers"
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

	// ---------- auth ----------
	authHandler := &auth.Handler{
		Store: sessionStore,
		RenderLogin: func(w http.ResponseWriter, r *http.Request, errMsg string) {
			pages.Login(errMsg).Render(r.Context(), w)
		},
	}

	// ---------- products ----------
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

	// ---------- inventory ----------
	invRepo := inventory.NewRepository(database.Pool)
	invHandler := &inventory.Handler{Repo: invRepo}

	invHandler.RenderList = func(w http.ResponseWriter, r *http.Request, rows []inventory.InventoryRow) {
		sess := auth.FromContext(r.Context())
		pages.InventoryPage(sess, rows).Render(r.Context(), w)
	}

	invHandler.RenderRow = func(w http.ResponseWriter, r *http.Request, row inventory.InventoryRow) {
		partials.InventoryRow(row).Render(r.Context(), w)
	}

	invHandler.RenderReceiveModal = func(w http.ResponseWriter, r *http.Request, row inventory.InventoryRow, suppliers []inventory.Supplier) {
		partials.ReceiveModal(row, suppliers).Render(r.Context(), w)
	}

	invHandler.RenderAdjustModal = func(w http.ResponseWriter, r *http.Request, row inventory.InventoryRow) {
		partials.AdjustModal(row).Render(r.Context(), w)
	}

	invHandler.RenderMovements = func(w http.ResponseWriter, r *http.Request, movements []inventory.Movement, filter inventory.MovementFilter) {
		sess := auth.FromContext(r.Context())
		pages.MovementsPage(sess, movements, filter).Render(r.Context(), w)
	}

	invHandler.RenderMovementsTable = func(w http.ResponseWriter, r *http.Request, movements []inventory.Movement) {
		partials.MovementsTable(movements).Render(r.Context(), w)
	}

	// ---------- suppliers ----------
	supRepo := suppliers.NewRepository(database.Pool)
	supHandler := &suppliers.Handler{Repo: supRepo}

	supHandler.RenderList = func(w http.ResponseWriter, r *http.Request, list []suppliers.SupplierWithStats) {
		// If htmx asked for it, return only the table partial (e.g. after Create).
		if r.Header.Get("HX-Request") == "true" {
			partials.SuppliersTable(list).Render(r.Context(), w)
			return
		}
		sess := auth.FromContext(r.Context())
		pages.SuppliersPage(sess, list).Render(r.Context(), w)
	}

	supHandler.RenderRow = func(w http.ResponseWriter, r *http.Request, s suppliers.SupplierWithStats) {
		partials.SupplierRow(s).Render(r.Context(), w)
	}

	supHandler.RenderNewModal = func(w http.ResponseWriter, r *http.Request, in suppliers.Input, errMsg string) {
		partials.SupplierNewModal(in, errMsg).Render(r.Context(), w)
	}

	supHandler.RenderEditModal = func(w http.ResponseWriter, r *http.Request, s suppliers.Supplier, errMsg string) {
		partials.SupplierEditModal(s, errMsg).Render(r.Context(), w)
	}

	supHandler.RenderDetail = func(w http.ResponseWriter, r *http.Request, s suppliers.Supplier, purchases []suppliers.PurchaseRow) {
		sess := auth.FromContext(r.Context())
		pages.SupplierDetailPage(sess, s, purchases).Render(r.Context(), w)
	}

	// ---------- router ----------
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

		// ---------- inventory routes ----------
		pr.Get("/inventory", invHandler.List)
		pr.Get("/inventory/receive/{productID}", invHandler.ReceiveModal)
		pr.Post("/inventory/receive/{productID}", invHandler.Receive)
		pr.Get("/inventory/adjust/{productID}", invHandler.AdjustModal)
		pr.Post("/inventory/adjust/{productID}", invHandler.Adjust)
		pr.Get("/inventory/movements", invHandler.Movements)

		// ---------- suppliers routes ----------
		pr.Get("/suppliers", supHandler.Index)
		pr.Get("/suppliers/new", supHandler.NewModal)
		pr.Post("/suppliers", supHandler.Create)
		pr.Get("/suppliers/{id}", supHandler.Detail)
		pr.Get("/suppliers/{id}/edit", supHandler.EditModal)
		pr.Put("/suppliers/{id}", supHandler.Update)
		pr.Delete("/suppliers/{id}", supHandler.Delete)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
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
