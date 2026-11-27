// Starts Swagger UI on the bot's own OpenAPI description. This is a separate file,
// not an inline script, because the page's Content-Security-Policy forbids inline
// scripts. Requests are same-origin, so the browser sends the session cookie from
// the !web login and "Try it out" works against the running bot.
window.ui = SwaggerUIBundle({
  // This script is at <base>/api/docs/assets/init.js, so the description is two levels up.
  url: new URL("../../v1/openapi.yaml", document.currentScript.src).pathname,
  dom_id: "#swagger-ui",
  deepLinking: true,
  presets: [SwaggerUIBundle.presets.apis],
  layout: "BaseLayout",
  tryItOutEnabled: true,
  requestInterceptor: function (req) {
    req.credentials = "same-origin";
    return req;
  },
});
