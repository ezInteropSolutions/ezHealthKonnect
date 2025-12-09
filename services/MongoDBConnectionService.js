// services/MongoDBConnectionService.js
// Node.js wrapper for MongoDB connection (used by MessageController for lineage queries)

const { MongoClient } = require('mongodb');

class MongoDBConnectionService {
    constructor() {
        this.client = null;
        this.db = null;
        this.isConnected = false;
    }

    async connect() {
        if (this.isConnected && this.client) {
            return this.db;
        }

        try {
            // Use Docker container name for MongoDB connection with authentication
            const uri = process.env.MONGODB_URI || process.env.MONGO_URI || 'mongodb://mongodb:27017/ezhealthkonnect';

            console.log('🔌 Connecting to MongoDB:', uri.replace(/mongodb:\/\/([^:]+:[^@]+@)?/, 'mongodb://***:***@'));

            this.client = new MongoClient(uri, {
                serverSelectionTimeoutMS: 5000,
                connectTimeoutMS: 10000
            });

            await this.client.connect();
            this.db = this.client.db('ezhealthkonnect');
            this.isConnected = true;

            console.log('✅ MongoDB connection established');
            return this.db;
        } catch (error) {
            console.error('❌ MongoDB connection failed:', error.message);
            this.isConnected = false;
            throw error;
        }
    }

    async disconnect() {
        if (this.client) {
            await this.client.close();
            this.isConnected = false;
            console.log('🔌 MongoDB connection closed');
        }
    }

    getCollection(name) {
        if (!this.db) {
            throw new Error('MongoDB not connected. Call connect() first.');
        }
        return this.db.collection(name);
    }
}

module.exports = MongoDBConnectionService;
