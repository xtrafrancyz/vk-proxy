var sqlite3 = require('sqlite3');

var db = new sqlite3.Database('database.db');

db.run('\
CREATE TABLE IF NOT EXISTS `users` (\
  `id` int(11) NOT NULL,\
  `name` varchar(255) DEFAULT NULL,\
  `surname` varchar(255) DEFAULT NULL,\
  `last_seen` int(11) DEFAULT NULL,\
  PRIMARY KEY (`id`)\
)');

module.exports = db;
