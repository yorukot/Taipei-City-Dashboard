/* global process */
import vue from "@vitejs/plugin-vue";
import { defineConfig } from "vite";
import viteCompression from "vite-plugin-compression";

// 嘗試讀取環境變數，若不存在則回傳 false
let isDockerCompose = process?.env.DOCKER_COMPOSE === "true";
const routingProxyTarget =
	process?.env.ROUTING_API_PROXY_TARGET ||
	(isDockerCompose
		? "http://host.docker.internal:8000"
		: "http://localhost:8000");

const routingProxyConfig = {
	target: routingProxyTarget,
	changeOrigin: true,
	rewrite: (path) => path.replace(/^\/routing/, ""),
};

let serverConfig = {
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

if (isDockerCompose) {
	serverConfig = {
		host: "0.0.0.0",
		port: 80, // 如有需要可變更 port
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

export default defineConfig({
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
	server: serverConfig,
});
