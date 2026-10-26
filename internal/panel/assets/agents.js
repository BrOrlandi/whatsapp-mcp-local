// The page of an agent outside this computer: it makes the agent's API key,
// shown once, and puts it in the texts to copy.
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

  // Step 1: a dialog with the key's name, the agent's, filled in. Once the
  // key is made, it shows once, goes into the texts to copy, and the next
  // steps appear.
  var dialog = document.querySelector("[data-api-key-dialog]");
  if (!dialog) return;
  var form = dialog.querySelector("[data-api-key-form]");
  var input = form.querySelector("input[name=name]");
  var submit = form.querySelector("[data-api-key-submit]");
  var spin = submit.querySelector(".spinner");
  var error = form.querySelector("[data-api-key-error]");
  var start = document.querySelector("[data-api-key-start]");
  var note = document.querySelector("[data-api-key-note]");
  var out = document.querySelector("[data-api-key-out]");
  var name = input.value;
  var busy = function (on) {
    submit.disabled = on;
    input.disabled = on;
    spin.hidden = !on;
  };

  document.querySelector("[data-api-key-open]").addEventListener("click", function () {
    input.value = name;
    error.hidden = true;
    busy(false);
    dialog.showModal();
    input.select();
  });
  dialog.querySelectorAll("[data-api-key-close]").forEach(function (button) {
    button.addEventListener("click", function () { if (!submit.disabled) dialog.close(); });
  });
  dialog.addEventListener("cancel", function (event) { if (submit.disabled) event.preventDefault(); });

  form.addEventListener("submit", function (event) {
    event.preventDefault();
    busy(true);
    error.hidden = true;
    call("POST", "/api/chaves", { name: input.value, tool: form.getAttribute("data-api-key-tool") }).then(function (d) {
      dialog.close();
      start.hidden = true;
      out.querySelector("[data-api-key-value]").textContent = d.key;
      out.hidden = false;
      document.querySelectorAll("[data-api-key-slot]").forEach(function (slot) {
        slot.textContent = slot.textContent.split("<chave>").join(d.key);
      });
      note.textContent = t("Chave criada. Copie agora: ela não aparece de novo.");
      note.hidden = false;
      var after = document.querySelectorAll("[data-api-key-after]");
      after.forEach(function (step) { step.hidden = false; });
      if (after.length) after[0].scrollIntoView({ behavior: "smooth", block: "start" });
    }).catch(function (err) {
      busy(false);
      error.textContent = err.message;
      error.hidden = false;
    });
  });
})();
