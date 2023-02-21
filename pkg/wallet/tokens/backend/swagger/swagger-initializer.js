window.onload = function() {
  //<editor-fold desc="Changeable Configuration Block">

  // the following lines will be replaced by docker/configurator, when it runs in a docker-container
  window.ui = SwaggerUIBundle({
    urls: [
      {name:"token wallet backend", url:"/api/v1/swagger/openapi_tokens.yaml"},
      {name:"money wallet backend", url:"https://dev-ab-money-backend.abdev1.guardtime.com/swagger/doc.json"}
    ],
    dom_id: '#swagger-ui',
    deepLinking: true,
    presets: [
      SwaggerUIBundle.presets.apis,
      SwaggerUIStandalonePreset
    ],
    plugins: [
      SwaggerUIBundle.plugins.DownloadUrl,
      SwaggerUIBundle.plugins.Topbar
    ],
    layout: "StandaloneLayout"
  });

  //</editor-fold>
};
