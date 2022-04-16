create table Repository (
    id int primary key,
    username text not null,
    name text not null
);

create table RepoStats (
    repo_id int REFERENCES Repository,
    date date not null default CURRENT_DATE,
    stars int not null,
    forks int not null,
    size int not null,
    subscribers int not null,
    primary key (repo_id, date)
);

create table RepoTrafficViews (
    repo_id int REFERENCES Repository,
    date date not null default CURRENT_DATE,
    count int not null,
    uniques int not null,
    primary key (repo_id, date)
);

create table RepoTrafficClones (
    repo_id int REFERENCES Repository,
    date date not null default CURRENT_DATE,
    count int not null,
    uniques int not null,
    primary key (repo_id, date)
);

create table RepoTrafficPaths (
    repo_id int REFERENCES Repository,
    date date not null default CURRENT_DATE,
    path text not null,
    title text not null,
    count int not null,
    uniques int not null,
    primary key (repo_id, date, path)
);

create table RepoTrafficReferrers (
    repo_id int REFERENCES Repository,
    date date not null default CURRENT_DATE,
    referrer text not null,
    count int not null,
    uniques int not null,
    primary key (repo_id, date, referrer)
);
