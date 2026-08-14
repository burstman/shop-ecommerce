window.trackAddToCart = function(id, name, price, currency) {
    currency = currency || 'TND';
    if (typeof fbq === 'function') {
        fbq('track', 'AddToCart', {
            content_ids: [id],
            content_name: name,
            content_type: 'product',
            value: price,
            currency: currency
        });
    }
};

window.trackInitiateCheckout = function(id, name, price, currency) {
    currency = currency || 'TND';
    if (typeof fbq === 'function') {
        fbq('track', 'InitiateCheckout', {
            content_ids: [id],
            content_name: name,
            content_type: 'product',
            value: price,
            currency: currency
        });
    }
};

window.trackPurchase = function(currency, value, trackValue) {
    currency = currency || 'TND';
    if (typeof fbq === 'function') {
        fbq('track', 'Purchase', { 
            currency: currency, 
            value: trackValue ? value : undefined 
        });
    }
};

window.closeQuickView = function() {
    const modal = document.getElementById('quick-view-modal');
    if (modal) modal.remove();
};
