const db = require('./config/database');
console.log('db keys:', Object.keys(db));
console.log('sequelize exists:', db.sequelize !== undefined);
console.log('sequelize type:', typeof db.sequelize);
