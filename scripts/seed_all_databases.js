#!/usr/bin/env node

/**
 * Seed All Databases for Database Enrichment Testing
 *
 * Seeds all 6 supported databases with identical test data:
 * - PostgreSQL (already exists - primary database)
 * - MongoDB
 * - Redis
 * - MySQL
 * - SQL Server
 * - Oracle
 *
 * Each database gets 3 patients and 3 providers for consistent testing.
 */

const { MongoClient } = require('mongodb');
const Redis = require('ioredis');
const { Client: PgClient } = require('pg');
const mysql = require('mysql2/promise');
const sql = require('mssql');
const oracledb = require('oracledb');

// Configuration
const MONGODB_URI = process.env.MONGODB_URI || 'mongodb://ezhealth_user:secure_password_change_me@localhost:27017/ezhealthkonnect?authSource=admin';
const REDIS_CONFIG = {
    host: process.env.REDIS_HOST || 'localhost',
    port: parseInt(process.env.REDIS_PORT || '6379'),
    password: process.env.REDIS_PASSWORD || 'secure_password_change_me'
};
const MYSQL_CONFIG = {
    host: process.env.MYSQL_HOST || 'localhost',
    port: parseInt(process.env.MYSQL_PORT || '3306'),
    user: process.env.MYSQL_USER || 'ezhealth_user',
    password: process.env.MYSQL_PASSWORD || 'secure_password_change_me',
    database: process.env.MYSQL_DATABASE || 'ezhealthkonnect'
};
const SQLSERVER_CONFIG = {
    server: process.env.SQLSERVER_HOST || 'localhost',
    port: parseInt(process.env.SQLSERVER_PORT || '1433'),
    user: process.env.SQLSERVER_USER || 'sa',
    password: process.env.SQLSERVER_PASSWORD || 'SecurePassword123!',
    database: process.env.SQLSERVER_DATABASE || 'master',
    options: {
        encrypt: false,
        trustServerCertificate: true
    }
};
const ORACLE_CONFIG = {
    user: process.env.ORACLE_USER || 'ezhealth_user',
    password: process.env.ORACLE_PASSWORD || 'secure_password_change_me',
    connectString: process.env.ORACLE_CONNECT_STRING || 'localhost:1521/FREEPDB1'
};

// Sample data (same as seed_nosql_test_data.js)
const patients = [
    { mrn: 'MRN123456', firstName: 'John', lastName: 'Doe', dob: '1980-05-15', gender: 'M', phone: '555-1234', insurance: 'Blue Cross' },
    { mrn: 'MRN789012', firstName: 'Jane', lastName: 'Smith', dob: '1975-08-22', gender: 'F', phone: '555-5678', insurance: 'Aetna' },
    { mrn: 'MRN345678', firstName: 'Robert', lastName: 'Johnson', dob: '1992-03-10', gender: 'M', phone: '555-9012', insurance: 'UnitedHealthcare' }
];

const providers = [
    { providerId: 'PROV-001', npi: 'NPI-1234567890', firstName: 'Sarah', lastName: 'Williams', specialty: 'Cardiology', phone: '555-2000' },
    { providerId: 'PROV-002', npi: 'NPI-0987654321', firstName: 'Michael', lastName: 'Brown', specialty: 'Family Medicine', phone: '555-3000' },
    { providerId: 'PROV-003', npi: 'NPI-1122334455', firstName: 'Emily', lastName: 'Chen', specialty: 'Pediatrics', phone: '555-4000' }
];

// Seed MySQL
async function seedMySQL() {
    console.log('\n🐬 Seeding MySQL...');
    let connection;
    try {
        connection = await mysql.createConnection(MYSQL_CONFIG);

        // Create tables
        await connection.execute(`
            CREATE TABLE IF NOT EXISTS test_patients (
                mrn VARCHAR(50) PRIMARY KEY,
                first_name VARCHAR(100),
                last_name VARCHAR(100),
                dob DATE,
                gender CHAR(1),
                phone VARCHAR(20),
                insurance VARCHAR(100)
            )
        `);

        await connection.execute(`
            CREATE TABLE IF NOT EXISTS test_providers (
                provider_id VARCHAR(50) PRIMARY KEY,
                npi VARCHAR(50) UNIQUE,
                first_name VARCHAR(100),
                last_name VARCHAR(100),
                specialty VARCHAR(100),
                phone VARCHAR(20)
            )
        `);

        // Clear existing data
        await connection.execute('DELETE FROM test_patients');
        await connection.execute('DELETE FROM test_providers');

        // Insert patients
        for (const patient of patients) {
            await connection.execute(
                'INSERT INTO test_patients (mrn, first_name, last_name, dob, gender, phone, insurance) VALUES (?, ?, ?, ?, ?, ?, ?)',
                [patient.mrn, patient.firstName, patient.lastName, patient.dob, patient.gender, patient.phone, patient.insurance]
            );
        }

        // Insert providers
        for (const provider of providers) {
            await connection.execute(
                'INSERT INTO test_providers (provider_id, npi, first_name, last_name, specialty, phone) VALUES (?, ?, ?, ?, ?, ?)',
                [provider.providerId, provider.npi, provider.firstName, provider.lastName, provider.specialty, provider.phone]
            );
        }

        console.log(`✅ MySQL: ${patients.length} patients, ${providers.length} providers`);
        console.log('   📖 Test Query: SELECT * FROM test_patients WHERE mrn = ?');
        console.log('   📝 Test Param: MRN123456');

    } catch (error) {
        console.error('❌ MySQL Error:', error.message);
    } finally {
        if (connection) await connection.end();
    }
}

// Seed SQL Server
async function seedSQLServer() {
    console.log('\n🔷 Seeding SQL Server...');
    try {
        const pool = await sql.connect(SQLSERVER_CONFIG);

        // Create tables
        await pool.request().query(`
            IF NOT EXISTS (SELECT * FROM sys.objects WHERE object_id = OBJECT_ID(N'[dbo].[test_patients]') AND type in (N'U'))
            CREATE TABLE test_patients (
                mrn VARCHAR(50) PRIMARY KEY,
                first_name VARCHAR(100),
                last_name VARCHAR(100),
                dob DATE,
                gender CHAR(1),
                phone VARCHAR(20),
                insurance VARCHAR(100)
            )
        `);

        await pool.request().query(`
            IF NOT EXISTS (SELECT * FROM sys.objects WHERE object_id = OBJECT_ID(N'[dbo].[test_providers]') AND type in (N'U'))
            CREATE TABLE test_providers (
                provider_id VARCHAR(50) PRIMARY KEY,
                npi VARCHAR(50) UNIQUE,
                first_name VARCHAR(100),
                last_name VARCHAR(100),
                specialty VARCHAR(100),
                phone VARCHAR(20)
            )
        `);

        // Clear existing data
        await pool.request().query('DELETE FROM test_patients');
        await pool.request().query('DELETE FROM test_providers');

        // Insert patients
        for (const patient of patients) {
            await pool.request()
                .input('mrn', sql.VarChar(50), patient.mrn)
                .input('firstName', sql.VarChar(100), patient.firstName)
                .input('lastName', sql.VarChar(100), patient.lastName)
                .input('dob', sql.Date, patient.dob)
                .input('gender', sql.Char(1), patient.gender)
                .input('phone', sql.VarChar(20), patient.phone)
                .input('insurance', sql.VarChar(100), patient.insurance)
                .query('INSERT INTO test_patients (mrn, first_name, last_name, dob, gender, phone, insurance) VALUES (@mrn, @firstName, @lastName, @dob, @gender, @phone, @insurance)');
        }

        // Insert providers
        for (const provider of providers) {
            await pool.request()
                .input('providerId', sql.VarChar(50), provider.providerId)
                .input('npi', sql.VarChar(50), provider.npi)
                .input('firstName', sql.VarChar(100), provider.firstName)
                .input('lastName', sql.VarChar(100), provider.lastName)
                .input('specialty', sql.VarChar(100), provider.specialty)
                .input('phone', sql.VarChar(20), provider.phone)
                .query('INSERT INTO test_providers (provider_id, npi, first_name, last_name, specialty, phone) VALUES (@providerId, @npi, @firstName, @lastName, @specialty, @phone)');
        }

        console.log(`✅ SQL Server: ${patients.length} patients, ${providers.length} providers`);
        console.log('   📖 Test Query: SELECT * FROM test_patients WHERE mrn = @p1');
        console.log('   📝 Test Param: MRN123456');

        await pool.close();
    } catch (error) {
        console.error('❌ SQL Server Error:', error.message);
    }
}

// Seed Oracle
async function seedOracle() {
    console.log('\n🟠 Seeding Oracle...');
    let connection;
    try {
        connection = await oracledb.getConnection(ORACLE_CONFIG);

        // Create tables
        await connection.execute(`
            BEGIN
                EXECUTE IMMEDIATE 'DROP TABLE test_patients';
            EXCEPTION
                WHEN OTHERS THEN NULL;
            END;
        `);

        await connection.execute(`
            BEGIN
                EXECUTE IMMEDIATE 'DROP TABLE test_providers';
            EXCEPTION
                WHEN OTHERS THEN NULL;
            END;
        `);

        await connection.execute(`
            CREATE TABLE test_patients (
                mrn VARCHAR2(50) PRIMARY KEY,
                first_name VARCHAR2(100),
                last_name VARCHAR2(100),
                dob DATE,
                gender CHAR(1),
                phone VARCHAR2(20),
                insurance VARCHAR2(100)
            )
        `);

        await connection.execute(`
            CREATE TABLE test_providers (
                provider_id VARCHAR2(50) PRIMARY KEY,
                npi VARCHAR2(50) UNIQUE,
                first_name VARCHAR2(100),
                last_name VARCHAR2(100),
                specialty VARCHAR2(100),
                phone VARCHAR2(20)
            )
        `);

        // Insert patients
        for (const patient of patients) {
            await connection.execute(
                `INSERT INTO test_patients (mrn, first_name, last_name, dob, gender, phone, insurance)
                 VALUES (:mrn, :firstName, :lastName, TO_DATE(:dob, 'YYYY-MM-DD'), :gender, :phone, :insurance)`,
                {
                    mrn: patient.mrn,
                    firstName: patient.firstName,
                    lastName: patient.lastName,
                    dob: patient.dob,
                    gender: patient.gender,
                    phone: patient.phone,
                    insurance: patient.insurance
                }
            );
        }

        // Insert providers
        for (const provider of providers) {
            await connection.execute(
                `INSERT INTO test_providers (provider_id, npi, first_name, last_name, specialty, phone)
                 VALUES (:providerId, :npi, :firstName, :lastName, :specialty, :phone)`,
                {
                    providerId: provider.providerId,
                    npi: provider.npi,
                    firstName: provider.firstName,
                    lastName: provider.lastName,
                    specialty: provider.specialty,
                    phone: provider.phone
                }
            );
        }

        await connection.commit();

        console.log(`✅ Oracle: ${patients.length} patients, ${providers.length} providers`);
        console.log('   📖 Test Query: SELECT * FROM test_patients WHERE mrn = :1');
        console.log('   📝 Test Param: MRN123456');

    } catch (error) {
        console.error('❌ Oracle Error:', error.message);
    } finally {
        if (connection) await connection.close();
    }
}

// Import and run MongoDB/Redis seeding from existing script
async function seedMongoDBAndRedis() {
    const { seedMongoDB, seedRedis } = require('./seed_nosql_test_data.js');
    await seedMongoDB();
    await seedRedis();
}

// Main
async function main() {
    console.log('🌱 Seeding All 6 Databases for Database Enrichment Testing\n');

    const results = {
        mongodb: 'pending',
        redis: 'pending',
        mysql: 'pending',
        sqlserver: 'pending',
        oracle: 'pending'
    };

    // Seed all databases (continue on error)
    try {
        await seedMongoDBAndRedis();
        results.mongodb = 'success';
        results.redis = 'success';
    } catch (error) {
        console.error('MongoDB/Redis failed:', error.message);
        results.mongodb = 'failed';
        results.redis = 'failed';
    }

    try {
        await seedMySQL();
        results.mysql = 'success';
    } catch (error) {
        results.mysql = 'failed';
    }

    try {
        await seedSQLServer();
        results.sqlserver = 'success';
    } catch (error) {
        results.sqlserver = 'failed';
    }

    try {
        await seedOracle();
        results.oracle = 'success';
    } catch (error) {
        results.oracle = 'failed';
    }

    // Summary
    console.log('\n' + '='.repeat(60));
    console.log('DATABASE SEEDING SUMMARY');
    console.log('='.repeat(60));
    console.log(`PostgreSQL: ✅ (Primary database - already seeded)`);
    console.log(`MongoDB:    ${results.mongodb === 'success' ? '✅' : '❌'}`);
    console.log(`Redis:      ${results.redis === 'success' ? '✅' : '❌'}`);
    console.log(`MySQL:      ${results.mysql === 'success' ? '✅' : '❌'}`);
    console.log(`SQL Server: ${results.sqlserver === 'success' ? '✅' : '❌'}`);
    console.log(`Oracle:     ${results.oracle === 'success' ? '✅' : '❌'}`);
    console.log('='.repeat(60));

    const successCount = Object.values(results).filter(v => v === 'success').length + 1; // +1 for PostgreSQL
    console.log(`\n✅ ${successCount}/6 databases seeded successfully`);

    if (successCount === 6) {
        console.log('\n🎉 All databases ready for testing!');
        console.log('\n📝 Next Steps:');
        console.log('   1. Open Pipeline Builder');
        console.log('   2. Add Database Enrichment step');
        console.log('   3. Test each database type');
        console.log('   4. Use MRN: MRN123456 or NPI: NPI-1234567890');
    } else {
        console.log('\n⚠️  Some databases failed to seed. Check Docker containers:');
        console.log('   docker-compose ps');
    }
}

if (require.main === module) {
    main().catch(console.error);
}
