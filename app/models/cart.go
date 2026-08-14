package models

type CartItem struct {
	Product  Product
	Quantity int
}

type Cart struct {
	Items map[uint]*CartItem
	Total int
}
