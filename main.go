package main

import (
	"bufio"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

type AuthToken struct {
	Token     string
	ExpiresAt time.Time
}

var activeTokens = make(map[string]int)

func generateAuthToken() (string, error) {
	bytes := make([]byte, 16)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
func isValidToken(token string) (int, bool) {
	userID, exists := activeTokens[token]
	return userID, exists
}
func main() {
	// Connection strings for read and write databases (replica and master)
	readConnStr := "postgres://postgres:tanaka2004%21@localhost:5432/tms?sslmode=disable"  // Connection to replica (read-only)
	writeConnStr := "postgres://postgres:tanaka2004%21@localhost:5432/tms?sslmode=disable" // Connection to master (write)

	// Retry connection logic
	const maxRetries = 3
	var readDB, writeDB *sql.DB
	var err error

	// Connect to the read replica and write master
	for i := 0; i < maxRetries; i++ {
		// Connect to read replica
		readDB, err = sql.Open("postgres", readConnStr)
		if err != nil {
			log.Println("Error initializing read database connection:", err)
			if i < maxRetries-1 {
				log.Println("Retrying to connect to the read database...")
				time.Sleep(2 * time.Second) // wait before retrying
				continue
			} else {
				log.Fatal("Unable to connect to the read database after retries:", err)
			}
		}
		readDB.SetMaxOpenConns(10)                  // maximum number of open connections to the read database (pooling)
		readDB.SetMaxIdleConns(5)                   // maximum number of idle connections to the read database (pooling)
		readDB.SetConnMaxLifetime(30 * time.Minute) // maximum lifetime of a connection to the read database

		// Connect to write master
		writeDB, err = sql.Open("postgres", writeConnStr)
		if err != nil {
			log.Println("Error initializing write database connection:", err)
			if i < maxRetries-1 {
				log.Println("Retrying to connect to the write database...")
				time.Sleep(2 * time.Second) // wait before retrying
				continue
			} else {
				log.Fatal("Unable to connect to the write database after retries:", err)
			}
		}
		writeDB.SetMaxOpenConns(10)                  // maximum number of open connections to the write database (pooling)
		writeDB.SetMaxIdleConns(5)                   // maximum number of idle connections to the write database (pooling)
		writeDB.SetConnMaxLifetime(30 * time.Minute) // maximum lifetime of a connection to the write database

		// Ping the read and write databases to ensure they are available
		if err = readDB.Ping(); err != nil {
			log.Println("Error pinging the read database:", err)
			if i < maxRetries-1 {
				log.Println("Retrying to ping the read database...")
				time.Sleep(2 * time.Second) // wait before retrying
				continue
			} else {
				log.Fatal("Unable to ping the read database after retries:", err)
			}
		}
		if err = writeDB.Ping(); err != nil {
			log.Println("Error pinging the write database:", err)
			if i < maxRetries-1 {
				log.Println("Retrying to ping the write database...")
				time.Sleep(2 * time.Second) // wait before retrying
				continue
			} else {
				log.Fatal("Unable to ping the write database after retries:", err)
			}
		}

		// Connection successful
		log.Println("Connected to the read and write databases successfully")
		break
	}

	defer readDB.Close()
	defer writeDB.Close()

	// Create the user and task tables if they don't exist
	createUserTable(writeDB)
	createTaskTable(writeDB)

	// Menu for user to choose options
	for {

		fmt.Println("\n=======Choose an option:==========")
		fmt.Println("1 - Sign Up")
		fmt.Println("2 - Log In")
		fmt.Println("3 - Forgot Password?")
		fmt.Println("4 - Exit")

		// Reading user choice
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Enter your choice: ")
		choiceInput, _ := reader.ReadString('\n')
		choiceInput = sanitizeInput(choiceInput)

		choice, err := strconv.Atoi(choiceInput)
		if err != nil {
			log.Println("Invalid input. Please enter a number.")
			continue
		}

		switch choice {
		case 1:
			// Handle user sign-up
			signUp(writeDB)

		case 2:
			// Handle user login and subsequent task menu
			loggedInUserID, token := logIn(readDB)
			if loggedInUserID > 0 && token != "" {
				taskMenu(writeDB, loggedInUserID)
			} else {
				fmt.Println("Login failed. Returning to main menu.")
			}
		case 3:
			// Handle forgotten password recovery
			forgotPassword(writeDB)

		case 4:
			// Exit the program gracefully
			fmt.Println("Exiting program...")
			os.Exit(0)

		default:
			// Handle invalid menu options
			fmt.Println("Invalid choice. Please try again.")
		}

	}
}
func ValidPassword(password string) error {
	// Ensure password length is at least 8 characters
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters long")
	}
	var hasLower, hasUpper, hasDigit, hasSpecial bool
	for _, ch := range password {
		switch {
		case ch >= 'a' && ch <= 'z':
			hasLower = true
		case ch >= 'A' && ch <= 'Z':
			hasUpper = true
		case ch >= '0' && ch <= '9':
			hasDigit = true
		case strings.ContainsAny(string(ch), "@$!%*?&"):
			hasSpecial = true
		}
	}
	if !hasLower {
		return errors.New("password must contain at least one lowercase letter")
	}
	if !hasUpper {
		return errors.New("password must contain at least one uppercase letter")
	}
	if !hasDigit {
		return errors.New("password must contain at least one digit")
	}
	if !hasSpecial {
		return errors.New("password must contain at least one special character")
	}

	return nil // Password is valid
}

func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// Function to hash the security question answers
func hashAnswer(answer string) (string, error) {
	hashedAnswer, err := bcrypt.GenerateFromPassword([]byte(answer), bcrypt.DefaultCost)
	return string(hashedAnswer), err
}

// Function to create the "user" table
func createUserTable(db *sql.DB) {
	query := `CREATE TABLE IF NOT EXISTS "user" (
		user_id SERIAL PRIMARY KEY,
		username VARCHAR(50) UNIQUE NOT NULL,
		password VARCHAR(255) NOT NULL,
		fanswer VARCHAR(255),
		sanswer VARCHAR(255)
	)`
	_, err := db.Exec(query)
	if err != nil {
		log.Fatal("Error creating user table:", err)
	}
}

// Function to create the "task" table
func createTaskTable(db *sql.DB) {
	query := `
	CREATE TABLE IF NOT EXISTS "task" (
		task_id SERIAL PRIMARY KEY,
		user_id INT NOT NULL REFERENCES "user"(user_id) ON DELETE CASCADE,
		title VARCHAR(50) NOT NULL,
		description TEXT,
		status CHAR(1) DEFAULT 'N', -- 'N' for Not Done, 'C' for Complete
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`

	// Execute the query to create the table
	_, err := db.Exec(query)
	if err != nil {
		log.Fatalf("Error creating task table: %v", err)
	} else {
		log.Println("Task table created or already exists.")
	}
}

// Modify signUp function to handle security questions
func signUp(db *sql.DB) {
	reader := bufio.NewReader(os.Stdin)

	// Prompt for username
	fmt.Print("Enter username: ")
	username, err := reader.ReadString('\n')
	if err != nil {
		log.Println("Error reading input:", err)
		return
	}
	username = sanitizeInput(username)

	// Check if username already exists
	var existingUserID int
	queryCheckUsername := `SELECT user_id FROM "user" WHERE username = $1`
	err = db.QueryRow(queryCheckUsername, username).Scan(&existingUserID)
	if err == nil {
		fmt.Println("Username already exists. Please choose another username.")
		return
	} else if err != sql.ErrNoRows {
		log.Println("Error checking for existing username:", err)
		return
	}

	// Loop until user provides a valid password
	var password string
	for {
		// Prompt for password
		fmt.Print("Enter password: ")
		password, err = reader.ReadString('\n')
		if err != nil {
			log.Println("Error reading password:", err)
			return
		}
		password = sanitizeInput(password)

		// Validate password strength
		if err := ValidPassword(password); err != nil {
			fmt.Println("Error:", err)
			fmt.Println("Please try again with a valid password.")
			continue // Loop back if password is invalid
		}
		break // Exit loop if password is valid
	}

	// Prompt for answers to security questions
	fmt.Print("What was the first concert you attended? ")
	firstConcertAnswer, err := reader.ReadString('\n')
	if err != nil {
		log.Println("Error reading input:", err)
		return
	}
	firstConcertAnswer = sanitizeInput(firstConcertAnswer)

	fmt.Print("Who is your favorite artist? ")
	favoriteArtistAnswer, err := reader.ReadString('\n')
	if err != nil {
		log.Println("Error reading input:", err)
		return
	}
	favoriteArtistAnswer = sanitizeInput(favoriteArtistAnswer)

	// Hash the answers
	hashedFirstConcertAnswer, err := hashAnswer(firstConcertAnswer)
	if err != nil {
		log.Println("Error hashing first concert answer:", err)
		return
	}

	hashedFavoriteArtistAnswer, err := hashAnswer(favoriteArtistAnswer)
	if err != nil {
		log.Println("Error hashing favorite artist answer:", err)
		return
	}

	// Hash the password
	hashedPassword, err := hashPassword(password)
	if err != nil {
		log.Println("Error hashing password:", err)
		return
	}

	// Insert into the database
	query := `
		INSERT INTO "user" (username, password, fanswer, sanswer)
		VALUES ($1, $2, $3, $4)`
	_, err = db.Exec(query, username, hashedPassword, hashedFirstConcertAnswer, hashedFavoriteArtistAnswer)
	if err != nil {
		log.Println("Error signing up:", err)
		fmt.Println("Error creating account. Please try again.")
		return
	}

	fmt.Println("Sign Up successful! You can now log in.")
}

func checkPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
func logIn(db *sql.DB) (int, string) {
	reader := bufio.NewReader(os.Stdin)

	var failedAttempts int
	const maxAttempts = 3
	const lockDuration = 10 * time.Second

	for {
		// Prompt for username
		fmt.Print("Enter username: ")
		username, err := reader.ReadString('\n')
		if err != nil {
			log.Println("Error reading username:", err)
			continue
		}
		username = strings.TrimSpace(sanitizeInput(username))

		// Prompt for password
		fmt.Print("Enter password: ")
		password, err := reader.ReadString('\n')
		if err != nil {
			log.Println("Error reading password:", err)
			continue
		}
		password = strings.TrimSpace(sanitizeInput(password))

		// Query for user data
		query := `SELECT user_id, password FROM "user" WHERE username = $1`
		row := db.QueryRow(query, username)

		var userID int
		var hashedPassword string
		err = row.Scan(&userID, &hashedPassword)
		if err == sql.ErrNoRows {
			fmt.Println("Invalid username or password.")
		} else if err != nil {
			log.Println("Database error:", err)
			continue
		} else if !checkPasswordHash(password, hashedPassword) {
			fmt.Println("Invalid username or password.")
		} else {
			// Successful login
			token, err := generateAuthToken()
			if err != nil {
				log.Println("Error generating authentication token:", err)

			}

			// Store the token
			activeTokens[token] = userID
			fmt.Printf("Welcome back, %s!\n", username)
			fmt.Println("Login successful! Your authentication token is:", token)
			return userID, token // Return the user ID upon successful login
		}

		// Increment failed attempts
		failedAttempts++
		if failedAttempts >= maxAttempts {
			fmt.Println("Too many failed attempts. Please try again after 10 seconds.")
			time.Sleep(lockDuration)
			failedAttempts = 0 // Reset failed attempts after lockout
		}
	}
}

func checkAnswerHash(answer, hashedAnswer string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedAnswer), []byte(answer))
	return err == nil
}
func forgotPassword(db *sql.DB) {
	reader := bufio.NewReader(os.Stdin)

	// Ask for username
	fmt.Print("Enter your username: ")
	username, err := reader.ReadString('\n')
	if err != nil {
		log.Println("Error reading username:", err)
		return
	}
	username = sanitizeInput(username)

	// Retrieve hashed answers from the database
	var hashedFirstConcertAnswer, hashedFavoriteArtistAnswer string
	query := `SELECT fanswer, sanswer FROM "user" WHERE username = $1`
	err = db.QueryRow(query, username).Scan(&hashedFirstConcertAnswer, &hashedFavoriteArtistAnswer)
	if err == sql.ErrNoRows {
		fmt.Println("Username not found.")
		return
	} else if err != nil {
		log.Println("Error querying database:", err)
		return
	}

	// Ask security questions
	fmt.Print("What was the first concert you attended? ")
	firstConcertAnswer, err := reader.ReadString('\n')
	if err != nil {
		log.Println("Error reading input:", err)
		return
	}
	firstConcertAnswer = sanitizeInput(firstConcertAnswer)

	fmt.Print("Who is your favorite artist? ")
	favoriteArtistAnswer, err := reader.ReadString('\n')
	if err != nil {
		log.Println("Error reading input:", err)
		return
	}
	favoriteArtistAnswer = sanitizeInput(favoriteArtistAnswer)

	// Check if answers are correct
	if checkAnswerHash(firstConcertAnswer, hashedFirstConcertAnswer) && checkAnswerHash(favoriteArtistAnswer, hashedFavoriteArtistAnswer) {
		// Proceed with password reset
		fmt.Print("Enter a new password: ")
		newPassword, err := reader.ReadString('\n')
		if err != nil {
			log.Println("Error reading password:", err)
			return
		}
		newPassword = sanitizeInput(newPassword)

		// Validate the new password
		if err := ValidPassword(newPassword); err != nil {
			fmt.Println("Error:", err)
			return
		}

		// Hash the new password
		hashedNewPassword, err := hashPassword(newPassword)
		if err != nil {
			log.Println("Error hashing password:", err)
			return
		}

		// Update the password in the database
		updateQuery := `UPDATE "user" SET password = $1 WHERE username = $2`
		_, err = db.Exec(updateQuery, hashedNewPassword, username)
		if err != nil {
			log.Println("Error updating password:", err)
			return
		}

		fmt.Println("Password reset successfully!")

		// Navigate back to the login screen
		fmt.Println("You can now log in with your new password.")
		logIn(db) // Call the login function to allow the user to log in
	} else {
		fmt.Println("Security question answers are incorrect.")
	}
}

// Task management menu
func taskMenu(db *sql.DB, userID int) {

	for {
		fmt.Println("\nTask Management Menu:")
		fmt.Println("---------------------------------")
		fmt.Println("1 - Create Task")
		fmt.Println("2 - View Tasks")
		fmt.Println("3 - Update Task")
		fmt.Println("4 - Delete Task")
		fmt.Println("5 - Logout")

		var choice int
		fmt.Print("Enter your choice: ")
		_, err := fmt.Scan(&choice)
		if err != nil {
			log.Println("Invalid input. Please enter a number.")
			continue
		}

		switch choice {
		case 1:
			createTask(db, userID)
		case 2:
			viewTasks(db, userID)
		case 3:
			updateTask(db, userID)
		case 4:
			deleteTask(db, userID)
		case 5:
			fmt.Println("Logging out...")
			return
		default:
			fmt.Println("Invalid choice. Please try again.")
		}
	}
}

func createTask(db *sql.DB, userID int) {
	reader := bufio.NewReader(os.Stdin)

	// Clear buffer
	reader.ReadString('\n')
	fmt.Println("---------------------------------")
	fmt.Print("Enter title: ")
	title, err := reader.ReadString('\n')
	if err != nil {
		log.Println("Error reading input:", err)
		return
	}
	title = sanitizeInput(strings.TrimSpace(title))

	fmt.Print("Enter description: ")
	description, err := reader.ReadString('\n')
	if err != nil {
		log.Println("Error reading input:", err)
		return
	}
	description = sanitizeInput(strings.TrimSpace(description))

	// Ask for status input
	fmt.Print("Enter task status (C for Complete, N for Not Done): ")
	status, err := reader.ReadString('\n')
	if err != nil {
		log.Println("Error reading input:", err)
		return
	}
	status = sanitizeInput(strings.TrimSpace(status))

	// Ensure valid status input ('C' or 'N')
	if status != "C" && status != "N" {
		fmt.Println("Invalid status. Please enter 'C' for Complete or 'N' for Not Done.")
		return
	}

	// Insert task into the database
	query := `
	INSERT INTO "task" (user_id, title, description, status) 
	VALUES ($1, $2, $3, $4)`
	_, execErr := db.Exec(query, userID, title, description, status)
	if execErr != nil {
		log.Println("Error creating task:", execErr)
		return
	}
	fmt.Println("Task created successfully!")
}

func viewTasks(db *sql.DB, userID int) {
	query := `SELECT task_id, title, description, status, created_at, updated_at FROM "task" WHERE user_id = $1`
	rows, err := db.Query(query, userID)
	if err != nil {
		log.Println("Error retrieving tasks:", err)
		return
	}
	defer rows.Close()

	fmt.Println("---------------------------------")
	fmt.Println("YOUR TASKS:")
	for rows.Next() {
		var taskID int
		var title, description, status string
		var createdAt, updatedAt string

		err := rows.Scan(&taskID, &title, &description, &status, &createdAt, &updatedAt)
		if err != nil {
			log.Println("Error scanning row:", err)
			continue
		}

		// Display the task details
		fmt.Printf(" ID: %d \n TITLE: %s \n DESCRIPTION: %s \n STATUS: %s \n CREATED: %s \n UPDATED: %s\n ---------------------------------\n", taskID, title, description, status, createdAt, updatedAt)
	}
}
func updateTask(db *sql.DB, userID int) {
	reader := bufio.NewReader(os.Stdin)

	// Clear buffer
	reader.ReadString('\n')

	// Ask for task ID
	fmt.Print("Enter task ID to update: ")
	taskIDInput, _ := reader.ReadString('\n')
	taskID, _ := strconv.Atoi(sanitizeInput(taskIDInput))

	// Check if taskID exists in the database
	var exists bool
	queryCheck := `SELECT EXISTS (SELECT 1 FROM task WHERE task_id = $1 AND user_id = $2)`
	err := db.QueryRow(queryCheck, taskID, userID).Scan(&exists)
	if err != nil {
		log.Println("Error checking task existence:", err)
		return
	}

	if !exists {
		fmt.Println("Task ID does not exist in the database.")
		return
	}

	// Ask the user what they want to update
	fmt.Println("What would you like to update?")
	fmt.Println("T: Title")
	fmt.Println("D: Description")
	fmt.Println("S: Status")
	fmt.Print("Enter your choice (T/D/S): ")
	updateChoice, _ := reader.ReadString('\n')
	updateChoice = sanitizeInput(updateChoice)

	var title, description, status string
	var queryParts []string
	var args []interface{}

	// Based on user choice, ask for the appropriate field to update
	switch updateChoice {
	case "T":
		fmt.Print("Enter new task title: ")
		title, _ = reader.ReadString('\n')
		title = sanitizeInput(title)
		queryParts = append(queryParts, "title = $1")
		args = append(args, title)

	case "D":
		fmt.Print("Enter new task description: ")
		description, _ = reader.ReadString('\n')
		description = sanitizeInput(description)
		queryParts = append(queryParts, "description = $1")
		args = append(args, description)

	case "S":
		fmt.Print("Enter new task status (C for Complete, N for Not Done): ")
		status, _ = reader.ReadString('\n')
		status = sanitizeInput(status)

		// Ensure valid status input ('C' or 'N')
		if status != "C" && status != "N" {
			fmt.Println("Invalid status. Please enter 'C' for Complete or 'N' for Not Done.")
			return
		}
		queryParts = append(queryParts, "status = $1")
		args = append(args, status)

	default:
		fmt.Println("Invalid choice. Please select either T, D, or S.")
		return
	}

	// Add 'updated_at' field to query
	queryParts = append(queryParts, "updated_at = CURRENT_TIMESTAMP")

	// Combine all query parts into the final query
	query := `UPDATE task SET ` + stringJoin(queryParts, ", ") + ` WHERE task_id = $2 AND user_id = $3`

	// Add the taskID and userID to the args slice
	args = append(args, taskID, userID)

	// Execute the update query
	_, err = db.Exec(query, args...)
	if err != nil {
		log.Println("Error updating task:", err)
		return
	}

	fmt.Println("Task updated successfully!")
}

// Helper function to join strings with commas (to replace string concatenation in the query)
func stringJoin(parts []string, delimiter string) string {
	var result string
	for i, part := range parts {
		if i > 0 {
			result += delimiter
		}
		result += part
	}
	return result
}

func deleteTask(db *sql.DB, userID int) {
	reader := bufio.NewReader(os.Stdin)
	reader.ReadString('\n') // Discard leftover newline from the menu choice

	fmt.Print("Enter task ID to delete: ")
	taskIDInput, _ := reader.ReadString('\n')
	taskID, _ := strconv.Atoi(sanitizeInput(taskIDInput))

	query := `DELETE FROM "task" WHERE task_id = $1 AND user_id = $2`
	_, err := db.Exec(query, taskID, userID)
	if err != nil {
		log.Println("Error deleting task:", err)
		return
	}
	fmt.Println("Task deleted successfully!")
}

// Helper function to sanitize user input
func sanitizeInput(input string) string {
	return strings.TrimSpace(input)
}
