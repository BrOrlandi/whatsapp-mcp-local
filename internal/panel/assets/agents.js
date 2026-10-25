// The page of an agent outside this computer: it makes the agent's API key,
// shown once, puts it in the text to copy, and revokes keys.
(function () {
  "use strict";

  function call(method, path, body) {
    var options = { method: method, credentials: "same-origin", headers: { Accept: "application/json" } };
    if (body !== undefined) {
      options.headers["Content-Type"] = "application/json";
      options.body = JSON.stringify(body);
    }
    return fetch(path, options).then(function (response) {
      return response.json().catch(function () { return {}; }).then(function (data) {
        if (!response.ok) throw new Error(data.error || t("erro %d", response.status));
        return data;
      });
    });
  }

  var make = document.querySelector("[data-api-key-new]");
  if (make) {
    var note = document.querySelector("[data-api-key-note]");
    var out = document.querySelector("[data-api-key-out]");
    make.addEventListener("click", function () {
      make.disabled = true;
      note.hidden = true;
      call("POST", "/api/chaves", { name: make.getAttribute("data-api-key-new") }).then(function (d) {
        out.querySelector("[data-api-key-value]").textContent = d.key;
        out.hidden = false;
        document.querySelectorAll("[data-api-key-slot]").forEach(function (slot) {
          slot.textContent = slot.textContent.split("<chave>").join(d.key);
        });
        note.textContent = t("Chave criada. Copie agora: ela não aparece de novo.");
        note.hidden = false;
      }).catch(function (err) {
        make.disabled = false;
        note.textContent = err.message;
        note.hidden = false;
      });
    });
  }

  document.querySelectorAll("[data-api-key-revoke]").forEach(function (button) {
    button.addEventListener("click", function () {
      if (!window.confirm(t("Revogar esta chave? O agente que a usa perde o acesso na hora."))) return;
      button.disabled = true;
      call("DELETE", "/api/chaves/" + encodeURIComponent(button.getAttribute("data-api-key-revoke"))).then(function () {
        window.location.reload();
      }).catch(function (err) {
        button.disabled = false;
        window.alert(err.message);
      });
    });
  });
})();
