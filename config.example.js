var config = module.exports = {};

config.host = '127.0.0.1';
config.port = 8881;

// Включает аналитику
config.analytics = false;

// Пишет в консоль все проксируемые запросы
config.logRequests = true;

config.domain = {};

// Домен, который нужно вписывать в "Домен API" в приложении
// Конечно же не забудьте проверить в браузере что он работает, должно выдавать "403 Forbidden"
config.domain.api = 'vk-api-proxy.example.com';

// Адрес для проксирования ресурсов с вк (картинок, музыки, видео), может быть другим доменом
config.domain.assets = 'vk-api-proxy.example.com/_';
