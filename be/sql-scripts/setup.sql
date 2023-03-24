DROP TABLE IF EXISTS user_match_fk;
DROP TABLE IF EXISTS game_match;
DROP TABLE IF EXISTS record;
DROP TABLE IF EXISTS user;
CREATE TABLE user (
    id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    username VARCHAR(255) UNIQUE NOT NULL,
    status VARCHAR(255) NOT NULL,
    password VARCHAR(255) NOT NULL
);
INSERT INTO user (username, status, password)
VALUES ('john_doe', 'active', 'mysecretpassword');
INSERT INTO user (username, status, password)
VALUES ('jane_doe', 'active', 'mysecretpassword2');
CREATE TABLE record (
    id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id INT NOT NULL,
    last_logged_in TIMESTAMP NOT NULL,
    FOREIGN KEY (user_id) REFERENCES user(id)
);
INSERT INTO record (user_id, last_logged_in)
VALUES (1, '2022-03-16 10:00:00');
-- maybe move winner id into foreigh key table
CREATE TABLE game_match (
    id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    color VARCHAR(255) NOT NULL,
    winner_id INT NOT NULL,
    start_time TIMESTAMP NOT NULL,
    -- completed BOOLEAN,
    FOREIGN KEY (winner_id) REFERENCES user(id)
);
INSERT INTO game_match (color, winner_id, start_time)
VALUES ('red', 1, '2022-03-16 10:00:00');
-- maybe add winner
CREATE TABLE user_match_fk (
    user_id INT NOT NULL,
    match_id INT NOT NULL,
    PRIMARY KEY (user_id, match_id),
    FOREIGN KEY (user_id) REFERENCES user(id),
    FOREIGN KEY (match_id) REFERENCES game_match(id)
);
INSERT INTO user_match_fk (user_id, match_id)
VALUES (2, 1);