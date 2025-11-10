// Quick fix for message_parser_service.go indentation issue
// This script is temporary - will be deleted after fix

const fs = require('fs');
const path = require('path');

const filePath = path.join(__dirname, 'services', 'message_parser_service.go');
let content = fs.readFileSync(filePath, 'utf8');

// Fix the indentation issue at lines 355-356
// Replace:
//   }
//   */
//   // END LEGACY CODE
// With:
//   }
//   	*/
//   	// END LEGACY CODE

content = content.replace(
  /(\t\})\n\*\/\n\/\/ END LEGACY CODE\n\}/m,
  '$1\n\t*/\n\t// END LEGACY CODE\n}'
);

fs.writeFileSync(filePath, content, 'utf8');
console.log('✅ Fixed indentation in message_parser_service.go');
