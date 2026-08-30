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
    var marker = document.getElementById("admin-global-poller");
    if (!marker) return;
    console.log("[chat-poll] initialized");
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
          if (el) {
            if (lastSidebarHTML && html !== lastSidebarHTML) playPing();
            lastSidebarHTML = html;
            var temp = document.createElement("div");
            temp.innerHTML = html;
            var items = temp.querySelectorAll("li");
            if (items.length) {
              el.innerHTML = "";
              items.forEach(function(li) { el.appendChild(li.cloneNode(true)); });
            }
          }
        }).catch(function(e) { console.error("[chat-poll] sidebar error:", e); });
    }
    function pollMessages() {
      if (!activeChatId) return;
      fetch("/admin/chat/" + activeChatId + "/messages?nocache=" + Date.now(), { headers: { "Cache-Control": "no-cache" } })
        .then(function(r) { return r.text(); })
        .then(function(html) {
          var el = document.getElementById("chat-messages-" + activeChatId);
          if (el) { el.innerHTML = html; el.scrollTop = el.scrollHeight; }
        }).catch(function(e) { console.error("[chat-poll] messages error:", e); });
    }
    updateActiveId();
    console.log("[chat-poll] polling started, activeChatId=" + activeChatId);
    setInterval(function() { updateActiveId(); pollSidebar(); pollMessages(); }, 3000);
  }
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", initAdminChatPolling);
  } else {
    initAdminChatPolling();
  }
})();
