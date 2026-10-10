// EMI Service: plans, convert txn -> EMI, schedule, pay, foreclose.
// EMI formula: E = P*r*(1+r)^n / ((1+r)^n - 1), r = APR/12/100
// Run: PORT=8004 go run ./services/emi-service
package main

import (
	"encoding/json"
	"math"
	"os"

	"github.com/vikasgavandi/legitpayz-emi-service/pkg/db"
	"github.com/vikasgavandi/legitpayz-emi-service/pkg/events"
	"github.com/vikasgavandi/legitpayz-emi-service/pkg/obs"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func emiAmount(principal int64, apr float64, n int) int64 {
	r := apr / 12.0 / 100.0
	if r == 0 {
		return principal / int64(n)
	}
	pow := math.Pow(1+r, float64(n))
	e := float64(principal) * r * pow / (pow - 1)
	return int64(math.Round(e))
}

func main() {
	database, err := db.Connect()
	if err != nil {
		panic(err)
	}
	defer database.Close()

	r := obs.Engine("emi")

	// Calculator: GET /emi/plans?amount=50000&apr=34 -> tenures 3/6/9/12
	r.GET("/emi/plans", func(c *gin.Context) {
		var amount int64 = 100000
		var apr float64 = 34.0
		if v := c.Query("amount"); v != "" {
			_, _ = parseInt(v, &amount)
		}
		if v := c.Query("apr"); v != "" {
			_, _ = parseFloat(v, &apr)
		}
		tenures := []int{3, 6, 9, 12, 18, 24}
		var out []map[string]any
		for _, n := range tenures {
			e := emiAmount(amount, apr, n)
			out = append(out, map[string]any{"tenure_months": n, "emi": e, "total": e * int64(n), "apr": apr})
		}
		c.JSON(200, out)
	})

	// Convert a posted PURCHASE txn into EMI
	r.POST("/emi/convert", func(c *gin.Context) {
		var req struct {
			CardID        string `json:"card_id" binding:"required"`
			TransactionID string `json:"transaction_id" binding:"required"`
			TenureMonths  int    `json:"tenure_months" binding:"required,min=3,max=24"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		var principal int64
		var typ string
		if err := database.QueryRow(`SELECT amount,txn_type FROM transactions WHERE id=$1 AND card_id=$2`, req.TransactionID, req.CardID).Scan(&principal, &typ); err != nil {
			c.JSON(404, gin.H{"error": "transaction not found"})
			return
		}
		if typ != "PURCHASE" {
			c.JSON(400, gin.H{"error": "only PURCHASE can be converted"})
			return
		}
		var apr float64
		_ = database.QueryRow(`SELECT p.interest_rate_apr FROM credit_cards cc JOIN card_products p ON p.code=cc.product_code WHERE cc.id=$1`, req.CardID).Scan(&apr)
		if apr == 0 {
			apr = 34.0
		}
		emi := emiAmount(principal, apr, req.TenureMonths)
		sched := make([]map[string]any, req.TenureMonths)
		for i := range sched {
			sched[i] = map[string]any{"month": i + 1, "amount": emi, "status": "DUE"}
		}
		sj, _ := json.Marshal(sched)
		id := uuid.New().String()
		_, err = database.Exec(`INSERT INTO emis(id,card_id,transaction_id,principal,tenure_months,interest_rate_apr,emi_amount,schedule) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
			id, req.CardID, req.TransactionID, principal, req.TenureMonths, apr, emi, string(sj))
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		_ = events.Default.Publish("emi.events", req.CardID, `{"event":"EMI_CREATED","id":"`+id+`"}`)
		c.JSON(201, gin.H{"emi_id": id, "emi_amount": emi, "tenure": req.TenureMonths, "apr": apr, "total": emi * int64(req.TenureMonths)})
	})

	r.GET("/emi/card/:id", func(c *gin.Context) {
		rows, _ := database.Query(`SELECT id,principal,tenure_months,interest_rate_apr,emi_amount,paid_months,status FROM emis WHERE card_id=$1 ORDER BY created_at DESC`, c.Param("id"))
		defer rows.Close()
		var out []map[string]any
		for rows.Next() {
			var id, st string
			var p, e int64
			var n, paid int
			var apr float64
			_ = rows.Scan(&id, &p, &n, &apr, &e, &paid, &st)
			out = append(out, map[string]any{"emi_id": id, "principal": p, "tenure": n, "apr": apr, "emi": e, "paid": paid, "status": st})
		}
		c.JSON(200, out)
	})

	// Pay one installment (also relieves used_limit + posts EMI_DEBIT txn)
	r.POST("/emi/:id/pay", func(c *gin.Context) {
		var emiID = c.Param("id")
		var cardID string
		var emiAmt int64
		var paid, tenure int
		var status string
		if err := database.QueryRow(`SELECT card_id,emi_amount,paid_months,tenure_months,status FROM emis WHERE id=$1`, emiID).Scan(&cardID, &emiAmt, &paid, &tenure, &status); err != nil {
			c.JSON(404, gin.H{"error": "emi not found"})
			return
		}
		if status != "ACTIVE" {
			c.JSON(400, gin.H{"error": "emi " + status})
			return
		}
		tx, _ := database.Begin()
		_, _ = tx.Exec(`INSERT INTO transactions(card_id,txn_type,amount,merchant) VALUES($1,'EMI_DEBIT',$2,$3)`, cardID, emiAmt, "EMI "+emiID[:8])
		_, _ = tx.Exec(`UPDATE credit_cards SET used_limit=GREATEST(0,used_limit-$1) WHERE id=$2`, emiAmt, cardID)
		paid++
		newStatus := "ACTIVE"
		if paid >= tenure {
			newStatus = "CLOSED"
		}
		_, _ = tx.Exec(`UPDATE emis SET paid_months=$1,status=$2 WHERE id=$3`, paid, newStatus, emiID)
		_ = tx.Commit()
		c.JSON(200, gin.H{"paid_months": paid, "status": newStatus})
	})

	r.POST("/emi/:id/foreclose", func(c *gin.Context) {
		_, _ = database.Exec(`UPDATE emis SET status='FORECLOSED' WHERE id=$1`, c.Param("id"))
		c.JSON(200, gin.H{"status": "FORECLOSED"})
	})

	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true, "svc": "emi"}) })

	port := os.Getenv("PORT")
	if port == "" {
		port = "8004"
	}
	_ = r.Run(":" + port)
}

func parseInt(s string, out *int64) (int64, bool) {
	var n int64
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, false
		}
		n = n*10 + int64(ch-'0')
	}
	*out = n
	return n, true
}

func parseFloat(s string, out *float64) (float64, bool) {
	var n float64
	var frac float64 = 0
	var div float64 = 1
	var dot bool
	for _, ch := range s {
		if ch == '.' {
			dot = true
			continue
		}
		if ch < '0' || ch > '9' {
			return 0, false
		}
		if !dot {
			n = n*10 + float64(ch-'0')
		} else {
			div *= 10
			frac = frac*10 + float64(ch-'0')
			_ = div
		}
	}
	n += frac / div
	*out = n
	return n, true
}
