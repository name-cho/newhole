package httpserver

const (
	proxyErrorTemplate = `<!DOCTYPE html>
<html lang="en">
	<head>
	<meta charset="utf-8" />
	<title>Newhole is running...</title>
	<style>
		html {
		height: 100%%;
		}
		body {
			max-height: 100%%;
			font-family: system-ui, -apple-system, "Segoe UI", Roboto, Ubuntu,
				Cantarell, "Noto Sans", sans-serif, BlinkMacSystemFont, "Segoe UI",
				Helvetica, Arial, sans-serif, "Apple Color Emoji", "Segoe UI Emoji",
				"Segoe UI Symbol";
			overflow-y: auto;
		}
		.container {
			text-align: center;
			width: 800px;
			height: fit-content;

			position: absolute;
			top: 0;
			bottom: 0;
			left: 0;
			right: 0;

			margin: auto;
		}
		.error {
			color: #fa383e;
		}
	</style>
	</head>
	<body>
	<div class="container">
		<img
		src="%s"
		width="500px"
		alt="Newhole"
		/>
		<h1>Congratulations, your tunnel is up and running!</h1>
		<p>
		However... it looks like the application you're trying to expose is not
		available.
		<br />
		<br />
		Original error: <em class="error">%s</em>
		<br />
		<br />
		<small>
			This request would normally end up with 502 status code.
			<br />
			Restart the tunnel with <code>--disable-proxy-error-page</code>
			to get the plain 502 page instead.
		</small>
		</p>
	</div>
	</body>
</html>
`
)
