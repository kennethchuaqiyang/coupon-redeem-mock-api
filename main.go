// Coupon Redemption mock API — one endpoint, for practice.
// Run with `go run main.go`, listens on :8081 by default (override with PORT).
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Coupon struct {
	Code            string     `json:"code"`
	OwnerCustomerID string     `json:"ownerCustomerId"`
	DiscountPercent int        `json:"discountPercent"`
	Redeemed        bool       `json:"redeemed"`
	RedeemedBy      string     `json:"redeemedBy,omitempty"`
	RedeemedAt      *time.Time `json:"redeemedAt,omitempty"`
}

var (
	mu sync.Mutex

	seedCoupons = map[string]Coupon{
		"WELCOME10": {Code: "WELCOME10", OwnerCustomerID: "cust_amy", DiscountPercent: 10},
		"FLASH5":    {Code: "FLASH5", OwnerCustomerID: "cust_amy", DiscountPercent: 5},
		"SAVE20":    {Code: "SAVE20", OwnerCustomerID: "cust_ben", DiscountPercent: 20},
	}

	coupons map[string]Coupon
)

func resetState() {
	coupons = map[string]Coupon{}
	for k, v := range seedCoupons {
		coupons[k] = v
	}
	// SAVE20 starts out already redeemed, by its own owner.
	t := time.Now().Add(-24 * time.Hour)
	c := coupons["SAVE20"]
	c.Redeemed = true
	c.RedeemedBy = "cust_ben"
	c.RedeemedAt = &t
	coupons["SAVE20"] = c
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func errJSON(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

var redeemPathRe = regexp.MustCompile(`^/api/coupons/([^/]+)/redeem$`)

func redeemHandler(w http.ResponseWriter, r *http.Request) {
	m := redeemPathRe.FindStringSubmatch(r.URL.Path)
	if m == nil {
		errJSON(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodPost {
		errJSON(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	code := m[1]

	actorID := strings.TrimSpace(r.Header.Get("x-customer-id"))
	if actorID == "" {
		errJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	mu.Lock()
	defer mu.Unlock()

	c, found := coupons[code]
	// A code that doesn't exist and a code that exists but belongs to
	// someone else look identical from the outside — don't leak which.
	if !found || c.OwnerCustomerID != actorID {
		errJSON(w, http.StatusNotFound, "Coupon not found")
		return
	}

	if c.Redeemed {
		errJSON(w, http.StatusConflict, "Coupon already redeemed")
		return
	}

	now := time.Now()
	c.Redeemed = true
	c.RedeemedBy = actorID
	c.RedeemedAt = &now
	coupons[code] = c

	writeJSON(w, http.StatusOK, map[string]any{
		"code":            c.Code,
		"discountPercent": c.DiscountPercent,
		"redeemed":        true,
		"redeemedAt":      now.Format(time.RFC3339),
	})
}

func debugCouponsHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()
	writeJSON(w, http.StatusOK, coupons)
}

func debugResetHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()
	resetState()
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"routes": []string{
			"POST /api/coupons/{code}/redeem   (header: x-customer-id)",
			"GET  /debug/coupons",
			"POST /debug/reset",
		},
	})
}

func main() {
	mu.Lock()
	resetState()
	mu.Unlock()

	http.HandleFunc("/api/coupons/", redeemHandler)
	http.HandleFunc("/debug/coupons", debugCouponsHandler)
	http.HandleFunc("/debug/reset", debugResetHandler)
	http.HandleFunc("/", rootHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	addr := ":" + port
	log.Println("coupon-redeem-mock-api listening on", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
