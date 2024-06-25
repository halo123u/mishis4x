CREATE TABLE records (
    id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id INT NOT NULL,
    last_logged_in TIMESTAMP NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id)
);