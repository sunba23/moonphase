package layout

// htmxConfig is rendered into the shared <head> as <meta name="htmx-config">.
//
// htmx 2.x, by default, does not swap the body of a 4xx/5xx response — it only
// fires htmx:responseError. Our form handlers re-render the form with a
// field-error message and return 422, so without this override the message is
// dropped and the user sees nothing. The extra "422" rule sits ahead of the
// catch-all "[45].." rule (first match wins), so a 422 body is swapped while
// every other 4xx/5xx still surfaces as an error only.
const htmxConfig = `{"responseHandling":[` +
	`{"code":"204","swap":false},` +
	`{"code":"[23]..","swap":true},` +
	`{"code":"422","swap":true},` +
	`{"code":"[45]..","swap":false,"error":true}]}`
