// routes/users.js
const bcrypt = require('bcryptjs');
const express = require('express');
const router = express.Router();

const userService = require('../services/userService');
const auditService = require('../services/auditService');
const { requireAuth, requireAdmin } = require('../middleware/auth');

// Debug route to test if users routes are working
router.get('/test', (req, res) => {
    res.json({ message: 'Users routes are working!' });
});

// GET /api/users - Get all users (admin only)
router.get('/', requireAuth, requireAdmin, async (req, res) => {
    try {
        console.log('📋 Fetching users list');
        
        const users = await userService.getAllUsers();
        
        await auditService.logEvent({
            userId: req.session.user.id,
            action: 'USERS_VIEWED',
            entityType: 'User',
            metadata: { 
                totalUsers: users.length,
                postgresqlUsers: users.filter(u => u.source === 'postgresql').length,
                jsonUsers: users.filter(u => u.source === 'json').length,
                requestedBy: req.session.user.email
            },
            ipAddress: req.clientIP,
            userAgent: req.get('User-Agent'),
            result: 'success',
            riskLevel: 'low'
        });
        
        // Return just the users array (not wrapped in an object)
        // This is what most frontends expect for .map() to work
        res.json(users);
        
    } catch (error) {
        console.error('❌ Error fetching users:', error);
        
        await auditService.logEvent({
            userId: req.session.user.id,
            action: 'USERS_VIEW_FAILED',
            entityType: 'User',
            metadata: { 
                error: error.message,
                requestedBy: req.session.user.email
            },
            ipAddress: req.clientIP,
            result: 'error',
            riskLevel: 'medium'
        });
        
        res.status(500).json({ 
            message: 'Failed to fetch users',
            error: process.env.NODE_ENV === 'development' ? error.message : 'Internal server error'
        });
    }
});

// POST /api/users - Create new user (admin only)
router.post('/', requireAuth, requireAdmin, async (req, res) => {
    try {
        console.log('📝 Creating new user:', req.body);
        
        const { email, password, name, role } = req.body;
        
        // Validate input
        if (!email || !password || !name || !role) {
            return res.status(400).json({ message: 'All fields are required' });
        }
        
        if (!['admin', 'user'].includes(role)) {
            return res.status(400).json({ message: 'Invalid role. Must be admin or user' });
        }
        
        const newUser = await userService.createUser(
            { email, password, name, role },
            req.session.user.email
        );
        
        await auditService.logEvent({
            userId: req.session.user.id,
            action: 'USER_CREATED',
            entityType: 'User',
            entityId: newUser.id,
            metadata: { 
                email: newUser.email,
                role: newUser.role,
                createdBy: req.session.user.email
            },
            ipAddress: req.clientIP,
            result: 'success',
            riskLevel: 'medium',
            complianceFlags: { user_management: true }
        });
        
        console.log('✅ User created successfully:', newUser.email);
        
        res.status(201).json({
            message: 'User created successfully',
            user: newUser
        });
        
    } catch (error) {
        console.error('❌ Error creating user:', error);
        
        await auditService.logEvent({
            userId: req.session.user.id,
            action: 'USER_CREATE_FAILED',
            entityType: 'User',
            metadata: { 
                error: error.message,
                attempted_email: req.body.email
            },
            ipAddress: req.clientIP,
            result: 'error',
            riskLevel: 'high'
        });
        
        const statusCode = error.message.includes('already exists') ? 400 : 500;
        res.status(statusCode).json({ 
            message: error.message.includes('already exists') ? error.message : 'Failed to create user'
        });
    }
});

// PUT /api/users/:id - Update user (admin only)
router.put('/:id', requireAuth, requireAdmin, async (req, res) => {
    try {
        const { id } = req.params;
        const { email, name, role, password } = req.body;
        
        console.log(`📝 Updating user ${id}:`, { email, name, role });
        
        // Find the user first
        const user = await userService.database.models.User.findByPk(id);
        if (!user) {
            return res.status(404).json({ message: 'User not found' });
        }
        
        // Prepare update data
        const updateData = {};
        if (email) updateData.email = email.toLowerCase();
        if (name) {
            const [firstName, ...lastNameParts] = name.split(' ');
            updateData.first_name = firstName;
            updateData.last_name = lastNameParts.join(' ') || '';
        }
        if (role) updateData.role = role;
        if (password) {
            updateData.password_hash = await bcrypt.hash(password, 12);
        }
        
        await user.update(updateData);
        
        await auditService.logEvent({
            userId: req.session.user.id,
            action: 'USER_UPDATED',
            entityType: 'User',
            entityId: id,
            metadata: { 
                updatedFields: Object.keys(updateData),
                updatedBy: req.session.user.email
            },
            ipAddress: req.clientIP,
            result: 'success',
            riskLevel: 'medium'
        });
        
        res.json({ message: 'User updated successfully' });
        
    } catch (error) {
        console.error('❌ Error updating user:', error);
        res.status(500).json({ message: 'Failed to update user' });
    }
});

// PATCH /api/users/:id/status - Toggle user status (admin only)
router.patch('/:id/status', requireAuth, requireAdmin, async (req, res) => {
    try {
        const { id } = req.params;
        const { status } = req.body;
        
        if (!['active', 'inactive'].includes(status)) {
            return res.status(400).json({ message: 'Invalid status' });
        }
        
        const user = await userService.database.models.User.findByPk(id);
        if (!user) {
            return res.status(404).json({ message: 'User not found' });
        }
        
        await user.update({ status });
        
        await auditService.logEvent({
            userId: req.session.user.id,
            action: 'USER_STATUS_CHANGED',
            entityType: 'User',
            entityId: id,
            metadata: { 
                newStatus: status,
                updatedBy: req.session.user.email
            },
            ipAddress: req.clientIP,
            result: 'success',
            riskLevel: 'medium'
        });
        
        res.json({ message: `User ${status === 'active' ? 'activated' : 'deactivated'} successfully` });
        
    } catch (error) {
        console.error('❌ Error updating user status:', error);
        res.status(500).json({ message: 'Failed to update user status' });
    }
});

// DELETE /api/users/:id - Delete user (admin only)
router.delete('/:id', requireAuth, requireAdmin, async (req, res) => {
    try {
        const { id } = req.params;
        
        const user = await userService.database.models.User.findByPk(id);
        if (!user) {
            return res.status(404).json({ message: 'User not found' });
        }
        
        // Prevent deleting yourself
        if (user.id === req.session.user.id) {
            return res.status(400).json({ message: 'Cannot delete your own account' });
        }
        
        await user.destroy();
        
        await auditService.logEvent({
            userId: req.session.user.id,
            action: 'USER_DELETED',
            entityType: 'User',
            entityId: id,
            metadata: { 
                deletedUser: user.email,
                deletedBy: req.session.user.email
            },
            ipAddress: req.clientIP,
            result: 'success',
            riskLevel: 'high'
        });
        
        res.json({ message: 'User deleted successfully' });
        
    } catch (error) {
        console.error('❌ Error deleting user:', error);
        res.status(500).json({ message: 'Failed to delete user' });
    }
});

module.exports = router;