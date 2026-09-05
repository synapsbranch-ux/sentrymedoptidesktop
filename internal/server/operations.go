package server

import "github.com/go-chi/chi/v5"

func (s *Server) registerOperationsRoutes(r chi.Router) {
	r.Get("/inventory", s.handleInventoryList)
	r.Post("/inventory", s.handleInventoryCreate)
	r.Post("/inventory/{id}/movements", s.handleStockMovement)
	r.Get("/inventory/{id}/movements", s.handleStockMovementsList)
	r.Get("/suppliers", s.handleSuppliersList)
	r.With(s.requireDoctor).Post("/suppliers", s.handleSupplierCreate)
	r.With(s.requireDoctor).Post("/purchase-orders", s.handlePurchaseOrderCreate)
	r.With(s.requireDoctor).Post("/purchase-orders/{id}/receive", s.handlePurchaseOrderReceive)
	r.Get("/invoices", s.handleInvoicesList)
	r.Get("/invoices/{id}", s.handleInvoiceGet)
	r.Post("/invoices", s.handleInvoiceCreate)
	r.Post("/invoices/{id}/payments", s.handlePaymentCreate)
	r.With(s.requireDoctor).Post("/payments/{id}/refunds", s.handleRefundCreate)
	r.Post("/pos/checkout", s.handlePOSCheckout)
	r.Get("/payment-methods", s.handlePaymentMethodsList)
	r.Get("/cash-register", s.handleCashRegisterCurrent)
	r.Post("/cash-register/open", s.handleCashRegisterOpen)
	r.Post("/cash-register/{id}/close", s.handleCashRegisterClose)
	r.Get("/lab-orders", s.handleLabOrdersList)
	r.Post("/lab-orders", s.handleLabOrderCreate)
	r.Patch("/lab-orders/{id}/status", s.handleLabOrderStatus)
	r.Post("/lab-orders/{id}/quality-control", s.handleLabQualityControl)
	r.With(s.requireDoctor).Get("/expenses", s.handleExpensesList)
	r.With(s.requireDoctor).Post("/expenses", s.handleExpenseCreate)
	r.With(s.requireDoctor).Get("/finance/summary", s.handleFinanceSummary)
	r.With(s.requireDoctor).Get("/reports/{report}", s.handleReport)
}
