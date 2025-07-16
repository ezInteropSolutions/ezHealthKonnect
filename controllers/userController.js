const userService = require('../services/userService');
const auditService = require('../services/auditService');
const { successResponse, errorResponse } = require('../utils/responses');

class UserController {
    async getUsers(req, res) {
        try {
            const users = await userService.getAllUsers();
            
            await auditService.logEvent({
                userId: req.session.user.id,
                action: 'USERS_VIEWED',
                entityType: 'User',
                metadata: { count: users.length },
                ipAddress: req.clientIP,
                result: 'success'
            });
            
            return successResponse(res, 'Users retrieved successfully', { users });
        } catch (error) {
            console.error('Error fetching users:', error);
            return errorResponse(res, 'Failed to fetch users', 500);
        }
    }

    async createUser(req, res) {
        try {
            const { email, password, name, role } = req.body;
            
            // Validation
            if (!email || !password || !name || !role) {
                return errorResponse(res, 'All fields are required', 400);
            }
            
            if (!['admin', 'user'].includes(role)) {
                return errorResponse(res, 'Invalid role. Must be admin or user', 400);
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
                result: 'success'
            });
            
            return successResponse(res, 'User created successfully', { user: newUser }, 201);
            
        } catch (error) {
            console.error('Error creating user:', error);
            
            await auditService.logEvent({
                userId: req.session.user.id,
                action: 'USER_CREATE_FAILED',
                entityType: 'User',
                metadata: { 
                    error: error.message,
                    attempted_email: req.body.email
                },
                ipAddress: req.clientIP,
                result: 'error'
            });
            
            return errorResponse(res, error.message.includes('already exists') ? error.message : 'Failed to create user', 
                               error.message.includes('already exists') ? 400 : 500);
        }
    }
}

module.exports = new UserController();