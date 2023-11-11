CREATE TABLE user_match_fk (
    user_id INT NOT NULL,
    match_id INT NOT NULL,
    PRIMARY KEY (user_id, match_id),
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (match_id) REFERENCES game_matches(id)
);