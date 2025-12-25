#!/usr/bin/env node

/**
 * Seed NoSQL Test Data
 *
 * Seeds MongoDB and Redis with test healthcare data for database enrichment testing.
 *
 * MongoDB Collections:
 * - patients: Patient demographics for EMPI lookups
 * - providers: Provider credentials for NPI/DEA/specialty lookups
 *
 * Redis Keys:
 * - patient:{mrn}: Cached patient data
 * - provider:{npi}: Cached provider data
 * - lab:ref-range:{test_code}: Lab reference ranges
 */

const { MongoClient } = require('mongodb');
const Redis = require('ioredis');

// Configuration from environment or defaults
const MONGODB_URI = process.env.MONGODB_URI || 'mongodb://ezhealth_user:secure_password_change_me@localhost:27017/ezhealthkonnect?authSource=admin';
const REDIS_HOST = process.env.REDIS_HOST || 'localhost';
const REDIS_PORT = process.env.REDIS_PORT || 6379;
const REDIS_PASSWORD = process.env.REDIS_PASSWORD || 'secure_password_change_me';

// Sample healthcare data
const samplePatients = [
    {
        mrn: 'MRN123456',
        ssn: '123-45-6789',
        firstName: 'John',
        lastName: 'Doe',
        dateOfBirth: '1980-05-15',
        gender: 'M',
        address: {
            street: '123 Main St',
            city: 'Springfield',
            state: 'IL',
            zip: '62701'
        },
        phone: '555-1234',
        email: 'john.doe@email.com',
        insurance: {
            provider: 'Blue Cross',
            memberId: 'BC-12345678',
            groupNumber: 'GRP-1000'
        },
        allergies: ['Penicillin', 'Peanuts'],
        chronicConditions: ['Hypertension', 'Type 2 Diabetes'],
        primaryProvider: 'NPI-1234567890',
        lastVisit: '2024-11-15'
    },
    {
        mrn: 'MRN789012',
        ssn: '987-65-4321',
        firstName: 'Jane',
        lastName: 'Smith',
        dateOfBirth: '1975-08-22',
        gender: 'F',
        address: {
            street: '456 Oak Ave',
            city: 'Chicago',
            state: 'IL',
            zip: '60601'
        },
        phone: '555-5678',
        email: 'jane.smith@email.com',
        insurance: {
            provider: 'Aetna',
            memberId: 'AET-87654321',
            groupNumber: 'GRP-2000'
        },
        allergies: ['Sulfa drugs'],
        chronicConditions: ['Asthma'],
        primaryProvider: 'NPI-0987654321',
        lastVisit: '2024-12-01'
    },
    {
        mrn: 'MRN345678',
        ssn: '555-55-5555',
        firstName: 'Robert',
        lastName: 'Johnson',
        dateOfBirth: '1992-03-10',
        gender: 'M',
        address: {
            street: '789 Elm St',
            city: 'Naperville',
            state: 'IL',
            zip: '60540'
        },
        phone: '555-9012',
        email: 'robert.j@email.com',
        insurance: {
            provider: 'UnitedHealthcare',
            memberId: 'UHC-11223344',
            groupNumber: 'GRP-3000'
        },
        allergies: [],
        chronicConditions: [],
        primaryProvider: 'NPI-1234567890',
        lastVisit: '2024-10-20'
    }
];

const sampleProviders = [
    {
        providerId: 'PROV-001',
        npi: 'NPI-1234567890',
        dea: 'DEA-AB1234567',
        firstName: 'Sarah',
        lastName: 'Williams',
        specialty: 'Internal Medicine',
        subspecialty: 'Cardiology',
        credentials: 'MD, FACC',
        phone: '555-2000',
        email: 'sarah.williams@hospital.com',
        hospitalAffiliations: ['Springfield General', 'Memorial Hospital'],
        acceptingNewPatients: true,
        languages: ['English', 'Spanish']
    },
    {
        providerId: 'PROV-002',
        npi: 'NPI-0987654321',
        dea: 'DEA-CD9876543',
        firstName: 'Michael',
        lastName: 'Brown',
        specialty: 'Family Medicine',
        subspecialty: null,
        credentials: 'DO',
        phone: '555-3000',
        email: 'michael.brown@clinic.com',
        hospitalAffiliations: ['Community Health Center'],
        acceptingNewPatients: true,
        languages: ['English']
    },
    {
        providerId: 'PROV-003',
        npi: 'NPI-1122334455',
        dea: 'DEA-EF1122334',
        firstName: 'Emily',
        lastName: 'Chen',
        specialty: 'Pediatrics',
        subspecialty: 'Neonatology',
        credentials: 'MD, FAAP',
        phone: '555-4000',
        email: 'emily.chen@childrens.org',
        hospitalAffiliations: ['Children\'s Hospital'],
        acceptingNewPatients: false,
        languages: ['English', 'Mandarin']
    }
];

// Lab reference ranges
const labReferenceRanges = {
    'CBC': {
        testCode: 'CBC',
        testName: 'Complete Blood Count',
        components: {
            WBC: { min: 4.5, max: 11.0, unit: 'K/uL' },
            RBC: { min: 4.5, max: 5.9, unit: 'M/uL' },
            HGB: { min: 13.5, max: 17.5, unit: 'g/dL' },
            HCT: { min: 41, max: 53, unit: '%' },
            PLT: { min: 150, max: 400, unit: 'K/uL' }
        }
    },
    'BMP': {
        testCode: 'BMP',
        testName: 'Basic Metabolic Panel',
        components: {
            Glucose: { min: 70, max: 100, unit: 'mg/dL' },
            Sodium: { min: 136, max: 145, unit: 'mmol/L' },
            Potassium: { min: 3.5, max: 5.0, unit: 'mmol/L' },
            Chloride: { min: 98, max: 107, unit: 'mmol/L' },
            CO2: { min: 23, max: 29, unit: 'mmol/L' },
            BUN: { min: 7, max: 20, unit: 'mg/dL' },
            Creatinine: { min: 0.6, max: 1.2, unit: 'mg/dL' }
        }
    },
    'HBA1C': {
        testCode: 'HBA1C',
        testName: 'Hemoglobin A1C',
        components: {
            A1C: { min: 4.0, max: 5.6, unit: '%', interpretation: { normal: '<5.7', prediabetes: '5.7-6.4', diabetes: '>=6.5' } }
        }
    }
};

async function seedMongoDB() {
    console.log('🍃 Connecting to MongoDB...');
    const client = new MongoClient(MONGODB_URI);

    try {
        await client.connect();
        console.log('✅ Connected to MongoDB');

        const db = client.db('ezhealthkonnect');

        // Seed patients collection
        console.log('\n📋 Seeding patients collection...');
        const patientsCollection = db.collection('patients');

        // Clear existing data
        await patientsCollection.deleteMany({});

        // Insert sample patients
        await patientsCollection.insertMany(samplePatients);
        console.log(`✅ Inserted ${samplePatients.length} patients`);

        // Create indexes
        await patientsCollection.createIndex({ mrn: 1 }, { unique: true });
        await patientsCollection.createIndex({ ssn: 1 });
        await patientsCollection.createIndex({ lastName: 1, firstName: 1 });
        console.log('✅ Created indexes on patients collection');

        // Seed providers collection
        console.log('\n👨‍⚕️ Seeding providers collection...');
        const providersCollection = db.collection('providers');

        // Clear existing data
        await providersCollection.deleteMany({});

        // Insert sample providers
        await providersCollection.insertMany(sampleProviders);
        console.log(`✅ Inserted ${sampleProviders.length} providers`);

        // Create indexes
        await providersCollection.createIndex({ npi: 1 }, { unique: true });
        await providersCollection.createIndex({ providerId: 1 }, { unique: true });
        await providersCollection.createIndex({ specialty: 1 });
        console.log('✅ Created indexes on providers collection');

        // Verify data
        console.log('\n🔍 Verifying MongoDB data...');
        const patientCount = await patientsCollection.countDocuments();
        const providerCount = await providersCollection.countDocuments();
        console.log(`   Patients: ${patientCount}`);
        console.log(`   Providers: ${providerCount}`);

        // Show sample query
        console.log('\n📖 Sample MongoDB Query:');
        console.log('   Collection: patients');
        console.log('   Filter: { "mrn": "{PID.3}" }');
        console.log('   Test with MRN: MRN123456');

    } catch (error) {
        console.error('❌ MongoDB Error:', error.message);
        throw error;
    } finally {
        await client.close();
        console.log('\n🔌 MongoDB connection closed');
    }
}

async function seedRedis() {
    console.log('\n🔴 Connecting to Redis...');

    const redis = new Redis({
        host: REDIS_HOST,
        port: REDIS_PORT,
        password: REDIS_PASSWORD
    });

    try {
        await redis.ping();
        console.log('✅ Connected to Redis');

        // Clear all existing keys (use with caution!)
        console.log('\n🧹 Clearing existing test data...');
        const keys = await redis.keys('patient:*');
        const providerKeys = await redis.keys('provider:*');
        const labKeys = await redis.keys('lab:*');

        if (keys.length > 0) await redis.del(...keys);
        if (providerKeys.length > 0) await redis.del(...providerKeys);
        if (labKeys.length > 0) await redis.del(...labKeys);
        console.log(`✅ Cleared ${keys.length + providerKeys.length + labKeys.length} keys`);

        // Seed patient data (using HSET for hash storage)
        console.log('\n📋 Seeding patient data...');
        for (const patient of samplePatients) {
            const key = `patient:${patient.mrn}`;

            // Store as JSON string
            await redis.set(key, JSON.stringify(patient), 'EX', 86400); // Expire in 24 hours

            // Also store as hash for HGETALL testing
            await redis.hset(`${key}:hash`, {
                mrn: patient.mrn,
                firstName: patient.firstName,
                lastName: patient.lastName,
                dob: patient.dateOfBirth,
                gender: patient.gender,
                phone: patient.phone,
                insurance: patient.insurance.provider
            });
        }
        console.log(`✅ Stored ${samplePatients.length} patients in Redis`);

        // Seed provider data
        console.log('\n👨‍⚕️ Seeding provider data...');
        for (const provider of sampleProviders) {
            const key = `provider:${provider.npi}`;

            // Store as JSON string
            await redis.set(key, JSON.stringify(provider), 'EX', 86400);

            // Also store as hash
            await redis.hset(`${key}:hash`, {
                npi: provider.npi,
                dea: provider.dea,
                firstName: provider.firstName,
                lastName: provider.lastName,
                specialty: provider.specialty,
                credentials: provider.credentials
            });
        }
        console.log(`✅ Stored ${sampleProviders.length} providers in Redis`);

        // Seed lab reference ranges
        console.log('\n🧪 Seeding lab reference ranges...');
        for (const [testCode, data] of Object.entries(labReferenceRanges)) {
            const key = `lab:ref-range:${testCode}`;
            await redis.set(key, JSON.stringify(data), 'EX', 604800); // Expire in 7 days
        }
        console.log(`✅ Stored ${Object.keys(labReferenceRanges).length} lab reference ranges`);

        // Verify data
        console.log('\n🔍 Verifying Redis data...');
        const patientKeys = await redis.keys('patient:*');
        const provKeys = await redis.keys('provider:*');
        const refRangeKeys = await redis.keys('lab:*');
        console.log(`   Patient keys: ${patientKeys.length / 2}`); // Divided by 2 because we have both JSON and hash
        console.log(`   Provider keys: ${provKeys.length / 2}`);
        console.log(`   Lab reference keys: ${refRangeKeys.length}`);

        // Show sample query
        console.log('\n📖 Sample Redis Query:');
        console.log('   Command: GET');
        console.log('   Key Pattern: patient:{PID.3}');
        console.log('   Test with: patient:MRN123456');

        // Test a sample query
        const sampleData = await redis.get('patient:MRN123456');
        console.log('\n✅ Sample query result:');
        console.log(JSON.stringify(JSON.parse(sampleData), null, 2));

    } catch (error) {
        console.error('❌ Redis Error:', error.message);
        throw error;
    } finally {
        redis.disconnect();
        console.log('\n🔌 Redis connection closed');
    }
}

async function main() {
    console.log('🌱 Starting NoSQL Test Data Seeding...\n');
    console.log('Configuration:');
    console.log(`   MongoDB: ${MONGODB_URI.replace(/:[^:]*@/, ':****@')}`);
    console.log(`   Redis: ${REDIS_HOST}:${REDIS_PORT}\n`);

    try {
        await seedMongoDB();
        await seedRedis();

        console.log('\n✅ All data seeded successfully!');
        console.log('\n📝 Next Steps:');
        console.log('   1. Refresh the Pipeline Builder UI');
        console.log('   2. Create a Database Enrichment step');
        console.log('   3. Select MongoDB or Redis as database type');
        console.log('   4. Configure connection and query');
        console.log('   5. Test with the Query Tester');
        console.log('\n🎯 Example MongoDB Configuration:');
        console.log('   Host: mongodb');
        console.log('   Port: 27017');
        console.log('   Database: ezhealthkonnect');
        console.log('   User: ezhealth_user');
        console.log('   Password: secure_password_change_me');
        console.log('   Collection: patients');
        console.log('   Filter: { "mrn": "{PID.3}" }');
        console.log('\n🎯 Example Redis Configuration:');
        console.log('   Host: redis');
        console.log('   Port: 6379');
        console.log('   Password: secure_password_change_me');
        console.log('   Command: GET');
        console.log('   Key: patient:{PID.3}');

    } catch (error) {
        console.error('\n❌ Seeding failed:', error.message);
        process.exit(1);
    }
}

// Run if called directly
if (require.main === module) {
    main();
}

module.exports = { seedMongoDB, seedRedis };
