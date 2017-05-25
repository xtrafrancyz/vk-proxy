var config = module.exports = {};

config.port = 8881;

config.domain = {};

// Домен, который нужно вписывать в "Домен API" в приложении
// Конечно же не забудьте проверить в браузере что он работает, должно выдавать "403 Forbidden"
config.domain.api = 'vk-api-proxy.example.com';

// Отдельный домен для проксирования ресурсов с вк (картинок, музыки, видео)
config.domain.assets = 'vk-assets-proxy.example.com';
