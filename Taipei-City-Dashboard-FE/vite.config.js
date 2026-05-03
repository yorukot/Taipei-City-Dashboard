/* global process */
import vue from "@vitejs/plugin-vue";
import { defineConfig, loadEnv } from "vite";
import viteCompression from "vite-plugin-compression";

function createServerConfig(mode) {
	const env = {
		...loadEnv(mode, process.cwd(), ""),
		...process.env,
	};
	const isDockerCompose = env.DOCKER_COMPOSE === "true";
	const routingProxyTarget =
		env.ROUTING_API_PROXY_TARGET ||
		env.VITE_ROUTING_API_PROXY_TARGET ||
		(isDockerCompose
			? "http://host.docker.internal:8000"
			: "http://localhost:8000");
	const routingProxyConfig = {
		target: routingProxyTarget,
		changeOrigin: true,
		headers: routingProxyTarget.includes("ngrok")
			? {
					"ngrok-skip-browser-warning": "true",
				}
			: {},
		rewrite: (path) => path.replace(/^\/routing/, ""),
	};

	if (isDockerCompose) {
		return {
			host: "0.0.0.0",
			port: 80,
			proxy: {
				"/api/dev": {
					target: "http://dashboard-be:8080",
					changeOrigin: true,
					rewrite: (path) => path.replace("/dev", "/v1"),
				},
				"/routing": routingProxyConfig,
			},
		};
	}

	return {
		host: "0.0.0.0",
		port: 80,
		proxy: {
			"/api": {
				target: "http://192.168.8.142:8080/api/dev",
				changeOrigin: true,
				secure: false,
				rewrite: (path) => path.replace(/^\/api/, ""),
			},
			"/geo_server": {
				target: "http://192.168.8.142:8080/geo_server/",
				changeOrigin: true,
				secure: false,
				rewrite: (path) => path.replace(/^\/geo_server/, ""),
			},
			"/routing": routingProxyConfig,
		},
	};
}

export default defineConfig(({ mode }) => ({
	plugins: [vue(), viteCompression()],
	build: {
		rollupOptions: {
			output: {
				manualChunks(id) {
					if (id.includes("node_modules")) {
						return id
							.toString()
							.split("node_modules/")[1]
							.split("/")[0]
							.toString();
					}
				},
			},
		},
		chunkSizeWarningLimit: 1600,
	},
	css: {
		preprocessorOptions: {
			scss: {
				api: "modern-compiler",
			},
		},
	},
	base: "/",
	server: createServerConfig(mode),
}));
