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

  // Admin Chat Polling — polls sidebar + messages every 3s on admin pages
  function initAdminChatPolling() {
    if (!document.getElementById("admin-global-poller")) return;
    var lastSidebarHTML = "";
    var activeChatId = null;
    function updateActiveId() {
      var match = window.location.pathname.match(/\/admin\/chat\/(\d+)/);
      activeChatId = match ? match[1] : null;
    }
    function playPing() {
      var audio = document.getElementById("chat-ping-sound");
      if (audio) { audio.currentTime = 0; audio.play().catch(function(){}); }
    }
    function pollSidebar() {
      fetch("/admin/chats/sidebar", { headers: { "Cache-Control": "no-cache" } })
        .then(function(r) { return r.text(); })
        .then(function(html) {
          var el = document.getElementById("sidebar-session-list");
          if (el && html !== lastSidebarHTML) {
            if (lastSidebarHTML !== "") playPing();
            lastSidebarHTML = html;
            el.outerHTML = html;
          }
        }).catch(function(){});
    }
    function pollMessages() {
      if (!activeChatId) return;
      fetch("/admin/chat/" + activeChatId + "/messages?nocache=" + Date.now(), { headers: { "Cache-Control": "no-cache" } })
        .then(function(r) { return r.text(); })
        .then(function(html) {
          var el = document.getElementById("chat-messages-" + activeChatId);
          if (el) el.innerHTML = html;
        }).catch(function(){});
    }
    updateActiveId();
    setInterval(function() { updateActiveId(); pollSidebar(); pollMessages(); }, 3000);
  }
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", initAdminChatPolling);
  } else {
    initAdminChatPolling();
  }
})();
