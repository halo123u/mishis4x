CREATE TABLE user (
    id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    username VARCHAR(255) UNIQUE NOT NULL,
    status VARCHAR(255) NOT NULL,
    password VARCHAR(255) NOT NULL
);
CREATE TABLE record (
    id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id INT NOT NULL,
    last_logged_in TIMESTAMP NOT NULL,
    FOREIGN KEY (user_id) REFERENCES user(id)
);

CREATE TABLE game_match (
    id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    color VARCHAR(255) NOT NULL,
    winner_id INT NOT NULL,
    start_time TIMESTAMP NOT NULL,
    -- completed BOOLEAN,
    FOREIGN KEY (winner_id) REFERENCES user(id)
);

CREATE TABLE user_match_fk (
    user_id INT NOT NULL,
    match_id INT NOT NULL,
    PRIMARY KEY (user_id, match_id),
    FOREIGN KEY (user_id) REFERENCES user(id),
    FOREIGN KEY (match_id) REFERENCES game_match(id)
);