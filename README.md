# vk-proxy
Прокси-сервер для приложения ВКонтакте на Android

## Настройка приложения
1. Открываем приложение ВК, заходим в Настройки
2. В конце списка настроек нажимаем О программе
3. Тыкаем на собаку раз 20
4. Открываем стандартный номеронабиратель и пишем следующий код: `*#*#856682583#*#*`
5. Нажимаем на Домен API, записываем туда `vk-api-proxy.xtrafrancyz.net` или ваш домен
6. Нажимаем на Домен OAuth, записываем туда `vk-oauth-proxy.xtrafrancyz.net` или ваш домен
7. Пользуемся приложением без проблем

## Запуск прокси
- Скопировать файл `config.example.js` в `config.json` и изменить нужные данные
- Выполнить `npm install`
- Выполнить `npm start`
- Настроить nginx по примеру в `conf/nginx.conf`
