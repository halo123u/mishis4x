CREATE TABLE game_matches (
    id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    color VARCHAR(255) NOT NULL,
    winner_id INT NOT NULL,
    start_time TIMESTAMP NOT NULL,
    -- completed BOOLEAN,
    FOREIGN KEY (winner_id) REFERENCES users(id)
);