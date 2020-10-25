const { createProxyMiddleware } = require('http-proxy-middleware');

module.exports = function(app) {
    app.use(
        '/gdxsv',
        createProxyMiddleware({
            target: 'http://zdxsv:9880',
            changeOrigin: true,
        })
    );
};