(() => {
  // app/assets/index.js
  window.trackAddToCart = function(id, name, price, currency) {
    currency = currency || "TND";
    if (typeof fbq === "function") {
      fbq("track", "AddToCart", {
        content_ids: [id],
        content_name: name,
        content_type: "product",
        value: price,
        currency
      });
    }
  };
  window.trackInitiateCheckout = function(id, name, price, currency) {
    currency = currency || "TND";
    if (typeof fbq === "function") {
      fbq("track", "InitiateCheckout", {
        content_ids: [id],
        content_name: name,
        content_type: "product",
        value: price,
        currency
      });
    }
  };
  window.trackPurchase = function(currency, value, trackValue) {
    currency = currency || "TND";
    if (typeof fbq === "function") {
      fbq("track", "Purchase", {
        currency,
        value: trackValue ? value : void 0
      });
    }
  };
  window.closeQuickView = function() {
    const modal = document.getElementById("quick-view-modal");
    if (modal) modal.remove();
  };
})();
