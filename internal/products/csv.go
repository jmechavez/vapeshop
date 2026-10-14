package products

import (
	"encoding/csv"
	"io"
	"strconv"
	"strings"
)

// Expected header row:
// sku,barcode,name,brand,version,flavor,category,cost_price,selling_price,minimum_stock
func parseCSV(r io.Reader) ([]*Product, error) {
	cr := csv.NewReader(r)
	cr.TrimLeadingSpace = true

	header, err := cr.Read()
	if err != nil {
		return nil, err
	}
	idx := map[string]int{}
	for i, h := range header {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}

	var out []*Product
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		get := func(name string) string {
			i, ok := idx[name]
			if !ok || i >= len(rec) {
				return ""
			}
			return strings.TrimSpace(rec[i])
		}
		p := &Product{
			SKU:          get("sku"),
			Barcode:      get("barcode"),
			Name:         get("name"),
			Brand:        get("brand"),
			Version:      get("version"),
			Flavor:       get("flavor"),
			Category:     get("category"),
			CostPrice:    atof(get("cost_price")),
			SellingPrice: atof(get("selling_price")),
			MinimumStock: atoi(get("minimum_stock")),
			Active:       true,
		}
		out = append(out, p)
	}
	return out, nil
}

func atof(s string) float64 { f, _ := strconv.ParseFloat(s, 64); return f }
func atoi(s string) int     { i, _ := strconv.Atoi(s); return i }
